package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	ctrBuilder "github.com/devrecon/ludus/internal/container"
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
}

// resolveBackend returns the effective backend, preferring CLI flag over config.
func resolveBackend() string {
	if backend != "" {
		return backend
	}
	return globals.Cfg.Engine.Backend
}

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

	if dockerbuild.IsContainerBackend(p.containerBackend) {
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
	} else {
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

	if dockerbuild.IsContainerBackend(p.containerBackend) {
		result, err := p.buildGameContainer(ctx, projectName)
		if err != nil {
			return err
		}
		p.serverBuildDir = result.OutputDir
		fmt.Printf("    %s server built in %s in %.0fs at %s\n", projectName, dockerbuild.ContainerCLI(p.containerBackend), result.Duration, result.OutputDir)
	} else {
		result, err := p.buildGameNative(ctx, projectName)
		if err != nil {
			return err
		}
		fmt.Printf("    %s server built in %.0fs at %s\n", projectName, result.Duration, result.OutputDir)
	}

	p.recordCache(cache.StageGameServer, p.serverHash)
	return nil
}

func (p *pipelineCtx) buildGameContainer(ctx context.Context, projectName string) (*gameBuilder.BuildResult, error) {
	engineImage, err := globals.ResolveEngineImage(p.cfg)
	if err != nil {
		return nil, err
	}
	builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
		EngineImage:   engineImage,
		ProjectPath:   p.cfg.Game.ProjectPath,
		ProjectName:   projectName,
		ServerTarget:  p.cfg.Game.ResolvedServerTarget(),
		GameTarget:    p.cfg.Game.ResolvedGameTarget(),
		ServerMap:     p.cfg.Game.ServerMap,
		EngineVersion: p.engineVersion,
		DDCMode:       p.ddcMode,
		DDCPath:       p.ddcPath,
		Runtime:       p.containerBackend,
	}, p.r)
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
		result, err = p.buildClientDocker(ctx, projectName)
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

func (p *pipelineCtx) buildClientDocker(ctx context.Context, projectName string) (*gameBuilder.ClientBuildResult, error) {
	engineImage, err := globals.ResolveEngineImage(p.cfg)
	if err != nil {
		return nil, err
	}
	builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
		EngineImage:    engineImage,
		ProjectPath:    p.cfg.Game.ProjectPath,
		ProjectName:    projectName,
		ClientTarget:   p.cfg.Game.ResolvedClientTarget(),
		ClientPlatform: "Linux",
		EngineVersion:  p.engineVersion,
		DDCMode:        p.ddcMode,
		DDCPath:        p.ddcPath,
		Runtime:        p.containerBackend,
	}, p.r)
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
