package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	ctrBuilder "github.com/devrecon/ludus/internal/container"
	"github.com/devrecon/ludus/internal/ddc"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/dflint"
	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/ecr"
	engBuilder "github.com/devrecon/ludus/internal/engine"
	gameBuilder "github.com/devrecon/ludus/internal/game"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/devrecon/ludus/internal/pricing"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/wsl"
)

// pipelineCtx holds shared state for pipeline stage execution.
type pipelineCtx struct {
	cfg              *config.Config
	r                *runner.Runner
	engineVersion    string
	containerBackend string
	ddcMode          string
	ddcPath          string
	arch             string
	serverBuildDir   string
	target           deploy.Target
	engineHash       string
	serverHash       string
	clientHash       string
	buildCache       *cache.Cache
	wslNative        bool
	wslDistro        string
}

// resolveBackend returns the effective backend, preferring CLI flag over config.
func resolveBackend() string { return globals.ResolveBackend(backend) }

// checkCacheSkip returns true if the stage can be skipped due to a cache hit.
// Prints cache status messages as a side effect.
func (p *pipelineCtx) checkCacheSkip(stage cache.StageKey, hash, label string) bool {
	if noCache {
		return false
	}
	if p.buildCache.IsHit(stage, hash) {
		fmt.Printf("    %s is up to date (cached), skipping.\n", label)
		return true
	}
	if reason := p.buildCache.MissReason(stage, hash); reason != "" {
		fmt.Printf("    Cache: %s\n", reason)
	}
	return false
}

// recordCache saves a cache entry for the given stage and hash.
func (p *pipelineCtx) recordCache(stage cache.StageKey, hash string) {
	p.buildCache.Set(stage, hash, time.Now().UTC().Format(time.RFC3339))
	_ = cache.Save(p.buildCache)
}

func (p *pipelineCtx) stageValidate(ctx context.Context) error {
	checker := prereq.NewChecker(p.cfg.Engine.SourcePath, p.cfg.Engine.Version, true, &p.cfg.Game)
	checker.Backend = p.containerBackend
	results := checker.RunAll()
	failed := 0
	for _, res := range results {
		marker := "[OK]"
		if !res.Passed {
			marker = "[FAIL]"
			failed++
		}
		fmt.Printf("    %-6s %s\n", marker, res.Name)
	}
	if failed > 0 {
		return fmt.Errorf("%d prerequisite check(s) failed", failed)
	}
	return nil
}

func (p *pipelineCtx) stageEngineBuild(ctx context.Context) error {
	if p.checkCacheSkip(cache.StageEngine, p.engineHash, "Engine build") {
		return nil
	}

	switch {
	case dockerbuild.IsContainerBackend(p.containerBackend):
		imageName := p.cfg.Engine.DockerImageName
		if imageName == "" {
			imageName = "ludus-engine"
		}

		// If a pre-built image is configured, skip engine build
		if p.cfg.Engine.DockerImage != "" {
			fmt.Printf("    Using pre-built engine image: %s\n", p.cfg.Engine.DockerImage)
			return nil
		}

		builder := dockerbuild.NewEngineImageBuilder(dockerbuild.EngineImageOptions{
			SourcePath: p.cfg.Engine.SourcePath,
			Version:    p.engineVersion,
			MaxJobs:    p.cfg.Engine.MaxJobs,
			ImageName:  imageName,
			BaseImage:  p.cfg.Engine.DockerBaseImage,
			Runtime:    p.containerBackend,
		}, p.r)
		result, err := builder.Build(ctx)
		if err != nil {
			return err
		}
		if err := state.UpdateEngineImage(&state.EngineImageState{
			ImageTag: result.ImageTag,
			BuiltAt:  time.Now().UTC().Format(time.RFC3339),
		}); err != nil {
			fmt.Printf("    Warning: failed to write state: %v\n", err)
		}
		cli := dockerbuild.ContainerCLI(p.containerBackend)
		fmt.Printf("    Engine %s image built in %.0fs: %s\n", cli, result.Duration, result.ImageTag)

	case dockerbuild.IsWSL2Backend(p.containerBackend):
		result, err := p.buildEngineWSL2(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("    Engine built in WSL2 in %.0fs: %s\n", result.Duration, result.EnginePath)

	default:
		builder := engBuilder.NewBuilder(engBuilder.BuildOptions{
			SourcePath: p.cfg.Engine.SourcePath,
			MaxJobs:    p.cfg.Engine.MaxJobs,
			Verbose:    globals.Verbose,
		}, p.r)
		result, err := builder.Build(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("    Engine built in %.0fs\n", result.Duration)
	}

	p.recordCache(cache.StageEngine, p.engineHash)
	return nil
}

func (p *pipelineCtx) stageGameBuild(ctx context.Context) error {
	projectName := p.cfg.Game.ProjectName

	if p.checkCacheSkip(cache.StageGameServer, p.serverHash, projectName+" server build") {
		return nil
	}

	switch {
	case dockerbuild.IsContainerBackend(p.containerBackend):
		result, err := p.buildGameContainer(ctx)
		if err != nil {
			return err
		}
		p.serverBuildDir = result.OutputDir
		fmt.Printf("    %s server built in %s in %.0fs at %s\n", projectName, dockerbuild.ContainerCLI(p.containerBackend), result.Duration, result.OutputDir)

	case dockerbuild.IsWSL2Backend(p.containerBackend):
		result, err := p.buildGameWSL2(ctx, projectName)
		if err != nil {
			return err
		}
		fmt.Printf("    %s server built in WSL2 in %.0fs at %s\n", projectName, result.Duration, result.OutputDir)

	default:
		result, err := p.buildGameNative(ctx, projectName)
		if err != nil {
			return err
		}
		fmt.Printf("    %s server built in %.0fs at %s\n", projectName, result.Duration, result.OutputDir)
	}

	p.recordCache(cache.StageGameServer, p.serverHash)
	return nil
}

func (p *pipelineCtx) buildGameContainer(ctx context.Context) (*gameBuilder.BuildResult, error) {
	engineImage, err := globals.ResolveEngineImage(p.cfg, false)
	if err != nil {
		return nil, err
	}
	opts := globals.BaseDockerGameOptions(p.cfg, engineImage, p.engineVersion, p.ddcMode, p.ddcPath, p.containerBackend)
	opts.ServerTarget = p.cfg.Game.ResolvedServerTarget()
	opts.GameTarget = p.cfg.Game.ResolvedGameTarget()
	opts.ServerMap = p.cfg.Game.ServerMap
	builder := dockerbuild.NewDockerGameBuilder(opts, p.r)
	return builder.Build(ctx)
}

func (p *pipelineCtx) buildGameNative(ctx context.Context, projectName string) (*gameBuilder.BuildResult, error) {
	builder := gameBuilder.NewBuilder(gameBuilder.BuildOptions{
		EnginePath:    p.cfg.Engine.SourcePath,
		ProjectPath:   p.cfg.Game.ProjectPath,
		ProjectName:   projectName,
		ServerTarget:  p.cfg.Game.ResolvedServerTarget(),
		GameTarget:    p.cfg.Game.ResolvedGameTarget(),
		Platform:      p.cfg.Game.Platform,
		Arch:          p.arch,
		ServerOnly:    true,
		ServerMap:     p.cfg.Game.ServerMap,
		EngineVersion: p.engineVersion,
		DDCMode:       p.ddcMode,
		DDCPath:       p.ddcPath,
	}, p.r)
	return builder.Build(ctx)
}

func (p *pipelineCtx) stageClientBuild(ctx context.Context) error {
	projectName := p.cfg.Game.ProjectName

	if p.checkCacheSkip(cache.StageGameClient, p.clientHash, projectName+" client build") {
		return nil
	}

	var result *gameBuilder.ClientBuildResult
	var err error
	var label string

	if dockerbuild.IsContainerBackend(p.containerBackend) {
		result, err = p.buildClientDocker(ctx)
		label = fmt.Sprintf("in %s ", dockerbuild.ContainerCLI(p.containerBackend))
	} else {
		result, err = p.buildClientNative(ctx, projectName)
	}
	if err != nil {
		return err
	}

	if err := state.UpdateClient(&state.ClientState{
		BinaryPath: result.ClientBinary,
		OutputDir:  result.OutputDir,
		Platform:   result.Platform,
		BuiltAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("    Warning: failed to write state: %v\n", err)
	}
	fmt.Printf("    %s client built %sin %.0fs at %s\n", projectName, label, result.Duration, result.OutputDir)

	p.recordCache(cache.StageGameClient, p.clientHash)
	return nil
}

func (p *pipelineCtx) buildClientDocker(ctx context.Context) (*gameBuilder.ClientBuildResult, error) {
	engineImage, err := globals.ResolveEngineImage(p.cfg, false)
	if err != nil {
		return nil, err
	}
	opts := globals.BaseDockerGameOptions(p.cfg, engineImage, p.engineVersion, p.ddcMode, p.ddcPath, p.containerBackend)
	opts.ClientTarget = p.cfg.Game.ResolvedClientTarget()
	opts.ClientPlatform = "Linux"
	builder := dockerbuild.NewDockerGameBuilder(opts, p.r)
	return builder.BuildClient(ctx)
}

func (p *pipelineCtx) buildClientNative(ctx context.Context, projectName string) (*gameBuilder.ClientBuildResult, error) {
	builder := gameBuilder.NewBuilder(gameBuilder.BuildOptions{
		EnginePath:    p.cfg.Engine.SourcePath,
		ProjectPath:   p.cfg.Game.ProjectPath,
		ProjectName:   projectName,
		ClientTarget:  p.cfg.Game.ResolvedClientTarget(),
		Platform:      p.cfg.Game.Platform,
		EngineVersion: p.engineVersion,
		DDCMode:       p.ddcMode,
		DDCPath:       p.ddcPath,
	}, p.r)
	return builder.BuildClient(ctx)
}

func (p *pipelineCtx) stageContainerBuild(ctx context.Context) error {
	containerHash := cache.ContainerKey(p.cfg, p.serverBuildDir)
	if p.checkCacheSkip(cache.StageContainerBuild, containerHash, "Container image") {
		return nil
	}

	builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
		ServerBuildDir: p.serverBuildDir,
		ImageName:      p.cfg.Container.ImageName,
		Tag:            p.cfg.Container.Tag,
		ServerPort:     p.cfg.Container.ServerPort,
		ProjectName:    p.cfg.Game.ProjectName,
		ServerTarget:   p.cfg.Game.ResolvedServerTarget(),
		Arch:           p.arch,
	}, p.r)
	result, err := builder.Build(ctx)
	if err != nil {
		return err
	}

	p.recordCache(cache.StageContainerBuild, containerHash)

	// Quick security lint of generated Dockerfile
	lintResult := dflint.LintDockerfile(builder.GenerateDockerfile())
	if lintResult.HasWarnings() {
		fmt.Printf("    Security: %s\n", lintResult.Summary())
	}

	fmt.Printf("    Image built: %s (%.0fs)\n", result.ImageTag, result.Duration)
	return nil
}

func (p *pipelineCtx) stageContainerPush(ctx context.Context) error {
	builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
		ImageName:    p.cfg.Container.ImageName,
		Tag:          p.cfg.Container.Tag,
		ServerPort:   p.cfg.Container.ServerPort,
		ProjectName:  p.cfg.Game.ProjectName,
		ServerTarget: p.cfg.Game.ResolvedServerTarget(),
		Arch:         p.arch,
	}, p.r)
	return builder.Push(ctx, ecr.PushOptions{
		ECRRepository: p.cfg.AWS.ECRRepository,
		AWSRegion:     p.cfg.AWS.Region,
		AWSAccountID:  p.cfg.AWS.AccountID,
		ImageTag:      p.cfg.Container.Tag,
	})
}

func (p *pipelineCtx) stageDeploy(ctx context.Context) error {
	if est := pricing.FormatEstimate(p.cfg.GameLift.InstanceType); est != "" {
		fmt.Printf("    %s\n", est)
	}
	if sug := pricing.FormatSuggestion(p.cfg.GameLift.InstanceType, p.arch); sug != "" {
		fmt.Printf("    %s\n", sug)
	}

	imageURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
		p.cfg.AWS.AccountID, p.cfg.AWS.Region, p.cfg.AWS.ECRRepository, p.cfg.Container.Tag)

	result, err := p.target.Deploy(ctx, deploy.DeployInput{
		ImageURI:       imageURI,
		ServerBuildDir: p.serverBuildDir,
		ServerPort:     p.cfg.Container.ServerPort,
	})
	if err != nil {
		return err
	}

	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: result.TargetName,
		Status:     result.Status,
		Detail:     result.Detail,
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("    Warning: failed to write state: %v\n", err)
	}

	fmt.Printf("    Deployed to %s: %s\n", result.TargetName, result.Detail)
	return nil
}

func (p *pipelineCtx) stageSession(ctx context.Context) error {
	sm, ok := p.target.(deploy.SessionManager)
	if !ok {
		return fmt.Errorf("target %q does not support game sessions", p.target.Name())
	}
	info, err := sm.CreateSession(ctx, 8)
	if err != nil {
		return err
	}
	if err := state.UpdateSession(&state.SessionState{
		SessionID: info.SessionID,
		IPAddress: info.IPAddress,
		Port:      info.Port,
	}); err != nil {
		fmt.Printf("    Warning: failed to write session state: %v\n", err)
	}
	fmt.Printf("    Game session created: %s\n", info.SessionID)
	fmt.Printf("    Connect: %s:%d\n", info.IPAddress, info.Port)
	return nil
}

// wsl2Fallback wraps a WSL2 init error with a Podman fallback recommendation.
func wsl2Fallback(err error) error {
	return fmt.Errorf("%w\n\nIf WSL2 is not available, use Podman instead:\n  ludus engine build --backend podman", err)
}

// buildEngineWSL2 compiles Unreal Engine inside a WSL2 distro.
// Mirrors the logic in cmd/engine/engine.go:runWSL2Build.
func (p *pipelineCtx) buildEngineWSL2(ctx context.Context) (*engBuilder.BuildResult, error) {
	w, err := wsl.New(p.r, p.wslDistro)
	if err != nil {
		return nil, wsl2Fallback(err)
	}
	fmt.Printf("    Using WSL2 distro: %s\n", w.Distro)

	sourcePath := p.cfg.Engine.SourcePath

	// Resolve WSL2 paths: --wsl-native syncs to ext4, otherwise uses /mnt/ virtiofs.
	wslEnginePath, wslDDCPath, err := p.resolveWSL2EnginePaths(ctx, w, sourcePath, p.engineVersion)
	if err != nil {
		return nil, err
	}

	result, err := wsl.BuildEngine(ctx, w, wsl.EngineOptions{
		SourcePath: sourcePath,
		MaxJobs:    p.cfg.Engine.MaxJobs,
		WSLNative:  p.wslNative,
		Version:    p.engineVersion,
	})
	if err != nil {
		return nil, err
	}

	// Persist engine location so game build can find it via state.Load().
	p.saveWSL2EngineState(wslEnginePath, wslDDCPath)
	return result, nil
}

// resolveWSL2EnginePaths returns the WSL2 engine and DDC paths for the build.
// When --wsl-native is set it rsyncs the source to native ext4 first (fast I/O);
// otherwise it converts the Windows source path to a /mnt/ virtiofs path.
func (p *pipelineCtx) resolveWSL2EnginePaths(ctx context.Context, w *wsl.WSL2, sourcePath, version string) (wslEnginePath, wslDDCPath string, err error) {
	if p.wslNative {
		fmt.Println("    Syncing engine source to WSL2 native ext4...")
		syncResult, syncErr := wsl.SyncEngine(ctx, p.r, w.Distro, wsl.SyncOptions{
			SourcePath: sourcePath,
			Version:    version,
		})
		if syncErr != nil {
			return "", "", syncErr
		}
		fmt.Printf("    Synced to %s in %.0fs\n", syncResult.WSLPath, syncResult.Duration.Seconds())
		return syncResult.WSLPath, syncResult.DDCPath, nil
	}
	// Default mode: convert Windows paths to /mnt/<drive>/ virtiofs mounts.
	enginePath := w.ToWSLPath(sourcePath)
	ddcPath := w.ToWSLPath(filepath.Join(filepath.Dir(sourcePath), ".ludus", "ddc"))
	return enginePath, ddcPath, nil
}

// saveWSL2EngineState persists the WSL2 engine and DDC paths to .ludus/state.json
// so that subsequent game builds can locate the engine without re-detection.
func (p *pipelineCtx) saveWSL2EngineState(wslEnginePath, wslDDCPath string) {
	syncTime := ""
	if p.wslNative {
		syncTime = time.Now().UTC().Format(time.RFC3339)
	}
	if err := state.UpdateWSL2Engine(&state.WSL2EngineState{
		EnginePath: wslEnginePath,
		IsNative:   p.wslNative,
		DDCPath:    wslDDCPath,
		SyncTime:   syncTime,
		BuiltAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("    Warning: failed to write state: %v\n", err)
	}
}

// buildGameWSL2 builds a dedicated server inside WSL2 using RunUAT.
// Mirrors the logic in cmd/game/game.go:runWSL2GameBuild.
func (p *pipelineCtx) buildGameWSL2(ctx context.Context, projectName string) (*gameBuilder.BuildResult, error) {
	// Load state written by buildEngineWSL2 to find the engine path.
	s, err := state.Load()
	if err != nil {
		return nil, fmt.Errorf("loading state: %w", err)
	}
	if s.WSL2Engine == nil {
		return nil, fmt.Errorf("no WSL2 engine build found; engine build stage must run first")
	}

	w, err := wsl.New(p.r, p.wslDistro)
	if err != nil {
		return nil, wsl2Fallback(err)
	}
	fmt.Printf("    Using WSL2 distro: %s\n", w.Distro)

	// Resolve DDC path: prefer the path from state (which reflects --wsl-native
	// if the engine was built that way), falling back to virtiofs host path.
	wslDDCPath := s.WSL2Engine.DDCPath
	if p.ddcMode == ddc.ModeLocal && wslDDCPath == "" {
		wslDDCPath = w.ToWSLPath(p.ddcPath)
	}

	opts := wsl.GameOptions{
		EnginePath:   s.WSL2Engine.EnginePath,
		ProjectPath:  p.cfg.Game.ProjectPath,
		ProjectName:  projectName,
		ServerTarget: p.cfg.Game.ResolvedServerTarget(),
		Platform:     p.cfg.Game.Platform,
		Arch:         p.arch,
		ServerMap:    p.cfg.Game.ServerMap,
		DDCMode:      p.ddcMode,
		DDCPath:      wslDDCPath,
	}

	return wsl.BuildGame(ctx, w, opts)
}
