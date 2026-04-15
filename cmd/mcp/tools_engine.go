package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/ecr"
	"github.com/devrecon/ludus/internal/engine"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/devrecon/ludus/internal/wsl"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type engineSetupInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type engineBuildInput struct {
	Jobs      int    `json:"jobs,omitempty" jsonschema:"Max parallel compile jobs (0 = auto-detect from RAM)"`
	Backend   string `json:"backend,omitempty" jsonschema:"Build backend: native, docker, podman, or wsl2 (default: from config)"`
	NoCache   bool   `json:"no_cache,omitempty" jsonschema:"Disable build caching (force rebuild even if inputs are unchanged)"`
	DryRun    bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
	WSLNative bool   `json:"wsl_native,omitempty" jsonschema:"Sync engine source to WSL2 native ext4 for faster builds (requires backend=wsl2)"`
	WSLDistro string `json:"wsl_distro,omitempty" jsonschema:"WSL2 distro override (default: first running WSL2 distro)"`
}

type engineResult struct {
	Success         bool    `json:"success"`
	EnginePath      string  `json:"engine_path,omitempty"`
	ImageTag        string  `json:"image_tag,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	Output          string  `json:"output,omitempty"`
	Error           string  `json:"error,omitempty"`
}

type enginePushInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type enginePushResult struct {
	Success  bool   `json:"success"`
	ImageTag string `json:"image_tag,omitempty"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
}

func registerEngineTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_engine_setup",
		Description: "Run Setup.sh to download Unreal Engine dependencies. Must be run before engine build.",
	}, handleEngineSetup)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_engine_build",
		Description: "Build Unreal Engine from source. Runs Setup, GenerateProjectFiles, and compiles ShaderCompileWorker + UnrealEditor. Use backend='podman' or 'docker' to build inside a container, or backend='wsl2' to build in a WSL2 Linux distro. This is a long-running operation.",
	}, handleEngineBuild)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_engine_push",
		Description: "Push engine container image to Amazon ECR. The image must have been previously built with backend='podman' or 'docker'.",
	}, handleEnginePush)
}

func handleEngineSetup(ctx context.Context, _ *mcp.CallToolRequest, input engineSetupInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg.Clone()
	r := newToolRunner(input.DryRun)

	b := engine.NewBuilder(engine.BuildOptions{
		SourcePath: cfg.Engine.SourcePath,
		Verbose:    true,
	}, r)

	var result engineResult
	result.EnginePath = cfg.Engine.SourcePath

	captured, err := withCapture(func() error {
		return b.Setup(ctx)
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = err.Error()
		return resultErr(result)
	}

	result.Success = true
	return resultOK(result)
}

func handleEngineBuild(ctx context.Context, _ *mcp.CallToolRequest, input engineBuildInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg.Clone()

	be := resolveBackend(input.Backend, cfg.Engine.Backend)

	if dockerbuild.IsContainerBackend(be) {
		return handleContainerEngineBuild(ctx, &cfg, input, be)
	}
	if dockerbuild.IsWSL2Backend(be) {
		return handleWSL2EngineBuild(ctx, &cfg, input)
	}

	engineHash := cache.EngineKey(&cfg)
	if hit := checkCacheHit(input.NoCache, cache.StageEngine, engineHash,
		engineResult{Success: true, EnginePath: cfg.Engine.SourcePath, Output: "Engine build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	r := newToolRunner(input.DryRun)

	jobs := input.Jobs
	if jobs == 0 {
		jobs = cfg.Engine.MaxJobs
	}

	b := engine.NewBuilder(engine.BuildOptions{
		SourcePath: cfg.Engine.SourcePath,
		MaxJobs:    jobs,
		Verbose:    true,
	}, r)

	var result engineResult
	result.EnginePath = cfg.Engine.SourcePath

	captured, err := withCapture(func() error {
		br, buildErr := b.Build(ctx)
		if br != nil {
			result.DurationSeconds = br.Duration
			result.Success = br.Success
		}
		return buildErr
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = fmt.Sprintf("engine build failed: %v", err)
		return resultErr(result)
	}

	if result.Success {
		saveCache(cache.StageEngine, engineHash)
	}

	return resultOK(result)
}

func handleContainerEngineBuild(ctx context.Context, cfg *config.Config, input engineBuildInput, be string) (*mcp.CallToolResult, any, error) {
	engineHash := cache.EngineKey(cfg)
	cli := dockerbuild.ContainerCLI(be)
	if hit := checkCacheHit(input.NoCache, cache.StageEngine, engineHash,
		engineResult{Success: true, EnginePath: cfg.Engine.SourcePath, Output: fmt.Sprintf("Engine %s build is up to date (cached), skipping.", cli)}); hit != nil {
		return hit, nil, nil
	}

	r := newToolRunner(input.DryRun)

	jobs := input.Jobs
	if jobs == 0 {
		jobs = cfg.Engine.MaxJobs
	}

	version, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
	imageName := cfg.Engine.DockerImageName
	if imageName == "" {
		imageName = "ludus-engine"
	}

	b := dockerbuild.NewEngineImageBuilder(dockerbuild.EngineImageOptions{
		SourcePath: cfg.Engine.SourcePath,
		Version:    version,
		MaxJobs:    jobs,
		ImageName:  imageName,
		BaseImage:  cfg.Engine.DockerBaseImage,
		NoCache:    input.NoCache,
		Runtime:    be,
	}, r)

	var result engineResult
	result.EnginePath = cfg.Engine.SourcePath

	captured, err := withCapture(func() error {
		br, buildErr := b.Build(ctx)
		if br != nil {
			result.DurationSeconds = br.Duration
			result.ImageTag = br.ImageTag
			result.Success = true
		}
		return buildErr
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = fmt.Sprintf("%s engine build failed: %v", cli, err)
		return resultErr(result)
	}

	// Persist engine image info to state
	if result.Success {
		_ = state.UpdateEngineImage(&state.EngineImageState{
			ImageTag: result.ImageTag,
			Version:  version,
			BuiltAt:  time.Now().UTC().Format(time.RFC3339),
		})
		saveCache(cache.StageEngine, engineHash)
	}

	return resultOK(result)
}

// resolveWSL2Paths resolves the engine and DDC paths for a WSL2 build.
// When wslNative is true, it syncs the source to native ext4; otherwise it
// uses the /mnt/ virtiofs path.
func resolveWSL2Paths(ctx context.Context, r *runner.Runner, w *wsl.WSL2, sourcePath, version string, wslNative bool) (enginePath, ddcPath string, err error) {
	if wslNative {
		syncResult, syncErr := wsl.SyncEngine(ctx, r, w.Distro, wsl.SyncOptions{
			SourcePath: sourcePath,
			Version:    version,
		})
		if syncErr != nil {
			return "", "", syncErr
		}
		return syncResult.WSLPath, syncResult.DDCPath, nil
	}
	ep := w.ToWSLPath(sourcePath)
	dp := w.ToWSLPath(filepath.Join(filepath.Dir(sourcePath), ".ludus", "ddc"))
	return ep, dp, nil
}

// saveWSL2EngineResult persists WSL2 engine build state and cache entry.
func saveWSL2EngineResult(enginePath, ddcPath, engineHash string, wslNative bool) {
	syncTime := ""
	if wslNative {
		syncTime = time.Now().UTC().Format(time.RFC3339)
	}
	_ = state.UpdateWSL2Engine(&state.WSL2EngineState{
		EnginePath: enginePath,
		IsNative:   wslNative,
		DDCPath:    ddcPath,
		SyncTime:   syncTime,
		BuiltAt:    time.Now().UTC().Format(time.RFC3339),
	})
	saveCache(cache.StageEngine, engineHash)
}

func handleWSL2EngineBuild(ctx context.Context, cfg *config.Config, input engineBuildInput) (*mcp.CallToolResult, any, error) {
	engineHash := cache.EngineKey(cfg)
	if hit := checkCacheHit(input.NoCache, cache.StageEngine, engineHash,
		engineResult{Success: true, EnginePath: cfg.Engine.SourcePath, Output: "Engine WSL2 build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	r := newToolRunner(input.DryRun)

	w, err := wsl.New(r, input.WSLDistro)
	if err != nil {
		return resultErr(engineResult{Error: fmt.Sprintf("WSL2 init failed: %v\n\nIf WSL2 is not available, use Podman instead: ludus_engine_build with backend=podman", err)})
	}

	version, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
	jobs := input.Jobs
	if jobs == 0 {
		jobs = cfg.Engine.MaxJobs
	}

	enginePath, ddcPath, err := resolveWSL2Paths(ctx, r, w, cfg.Engine.SourcePath, version, input.WSLNative)
	if err != nil {
		return resultErr(engineResult{Error: fmt.Sprintf("WSL2 sync failed: %v", err)})
	}

	var result engineResult
	result.EnginePath = cfg.Engine.SourcePath

	captured, err := withCapture(func() error {
		br, buildErr := wsl.BuildEngine(ctx, w, wsl.EngineOptions{
			SourcePath: cfg.Engine.SourcePath,
			MaxJobs:    jobs,
			WSLNative:  input.WSLNative,
			Version:    version,
		})
		if br != nil {
			result.DurationSeconds = br.Duration
			result.Success = br.Success
		}
		return buildErr
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = fmt.Sprintf("WSL2 engine build failed: %v", err)
		return resultErr(result)
	}

	if result.Success {
		saveWSL2EngineResult(enginePath, ddcPath, engineHash, input.WSLNative)
	}

	return resultOK(result)
}

func handleEnginePush(ctx context.Context, _ *mcp.CallToolRequest, input enginePushInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg
	r := newToolRunner(input.DryRun)

	imageTag, err := globals.ResolveEngineImage(cfg, false)
	if err != nil {
		return resultErr(enginePushResult{Error: err.Error()})
	}

	imageName := cfg.Engine.DockerImageName
	if imageName == "" {
		imageName = "ludus-engine"
	}

	b := dockerbuild.NewEngineImageBuilder(dockerbuild.EngineImageOptions{
		ImageName: imageName,
	}, r)

	var result enginePushResult
	result.ImageTag = imageTag

	captured, err := withCapture(func() error {
		return b.Push(ctx, ecr.PushOptions{
			ECRRepository: imageName,
			AWSRegion:     cfg.AWS.Region,
			AWSAccountID:  cfg.AWS.AccountID,
			ImageTag:      imageTag,
		})
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = fmt.Sprintf("engine push failed: %v", err)
		return resultErr(result)
	}

	result.Success = true
	return resultOK(result)
}
