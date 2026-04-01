package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/ecr"
	"github.com/devrecon/ludus/internal/engine"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type engineSetupInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type engineBuildInput struct {
	Jobs    int    `json:"jobs,omitempty" jsonschema:"Max parallel compile jobs (0 = auto-detect from RAM)"`
	Backend string `json:"backend,omitempty" jsonschema:"Build backend: native or docker (default: from config)"`
	NoCache bool   `json:"no_cache,omitempty" jsonschema:"Disable build caching (force rebuild even if inputs are unchanged)"`
	DryRun  bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
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
		Description: "Build Unreal Engine from source. Runs Setup, GenerateProjectFiles, and compiles ShaderCompileWorker + UnrealEditor. Use backend='docker' to build inside a Docker container. This is a long-running operation.",
	}, handleEngineBuild)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_engine_push",
		Description: "Push engine Docker image to Amazon ECR. The image must have been previously built with backend='docker'.",
	}, handleEnginePush)
}

func handleEngineSetup(ctx context.Context, _ *mcp.CallToolRequest, input engineSetupInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg
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
	cfg := *globals.Cfg

	be := resolveBackend(input.Backend, cfg.Engine.Backend)

	if be == "docker" {
		return handleDockerEngineBuild(ctx, &cfg, input)
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

func handleDockerEngineBuild(ctx context.Context, cfg *config.Config, input engineBuildInput) (*mcp.CallToolResult, any, error) {
	engineHash := cache.EngineKey(cfg)
	if hit := checkCacheHit(input.NoCache, cache.StageEngine, engineHash,
		engineResult{Success: true, EnginePath: cfg.Engine.SourcePath, Output: "Engine Docker build is up to date (cached), skipping."}); hit != nil {
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
		result.Error = fmt.Sprintf("docker engine build failed: %v", err)
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

func handleEnginePush(ctx context.Context, _ *mcp.CallToolRequest, input enginePushInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg
	r := newToolRunner(input.DryRun)

	// Resolve engine image tag
	imageTag := ""
	imageName := cfg.Engine.DockerImageName
	if imageName == "" {
		imageName = "ludus-engine"
	}

	s, err := state.Load()
	if err == nil && s.EngineImage != nil {
		imageTag = s.EngineImage.ImageTag
	}

	if imageTag == "" {
		version, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
		tag := version
		if tag == "" {
			tag = "latest"
		}
		imageTag = fmt.Sprintf("%s:%s", imageName, tag)
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
