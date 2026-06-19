package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
	gameBuilder "github.com/jpvelasco/ludus/internal/game"
	"github.com/jpvelasco/ludus/internal/state"
)

func (p *pipelineCtx) stageGameBuild(ctx context.Context) error {
	projectName := p.cfg.Game.ProjectName
	if p.checkCacheSkip(cache.StageGameServer, p.serverHash, projectName+" server build") {
		return nil
	}
	if err := p.dispatchGameBuild(ctx, projectName); err != nil {
		return err
	}
	p.recordCache(cache.StageGameServer, p.serverHash)
	return nil
}

func (p *pipelineCtx) dispatchGameBuild(ctx context.Context, projectName string) error {
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
	return nil
}

func (p *pipelineCtx) baseDockerGameOpts() (dockerbuild.DockerGameOptions, error) {
	engineImage, err := globals.ResolveEngineImage(p.cfg, false)
	if err != nil {
		return dockerbuild.DockerGameOptions{}, err
	}
	return globals.BaseDockerGameOptions(p.cfg, engineImage, p.engineVersion, p.ddcMode, p.ddcPath, p.ddcZenPath, p.containerBackend), nil
}

func (p *pipelineCtx) buildGameContainer(ctx context.Context) (*gameBuilder.BuildResult, error) {
	opts, err := p.baseDockerGameOpts()
	if err != nil {
		return nil, err
	}
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

	result, label, err := p.dispatchClientBuild(ctx, projectName)
	if err != nil {
		return err
	}

	p.saveClientState(result)
	fmt.Printf("    %s client built %sin %.0fs at %s\n", projectName, label, result.Duration, result.OutputDir)
	p.recordCache(cache.StageGameClient, p.clientHash)
	return nil
}

func (p *pipelineCtx) dispatchClientBuild(ctx context.Context, projectName string) (*gameBuilder.ClientBuildResult, string, error) {
	if dockerbuild.IsContainerBackend(p.containerBackend) {
		result, err := p.buildClientDocker(ctx)
		return result, fmt.Sprintf("in %s ", dockerbuild.ContainerCLI(p.containerBackend)), err
	}
	result, err := p.buildClientNative(ctx, projectName)
	return result, "", err
}

func (p *pipelineCtx) saveClientState(result *gameBuilder.ClientBuildResult) {
	if err := state.UpdateClient(&state.ClientState{
		BinaryPath: result.ClientBinary,
		OutputDir:  result.OutputDir,
		Platform:   result.Platform,
		BuiltAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("    Warning: failed to write state: %v\n", err)
	}
}

func (p *pipelineCtx) buildClientDocker(ctx context.Context) (*gameBuilder.ClientBuildResult, error) {
	opts, err := p.baseDockerGameOpts()
	if err != nil {
		return nil, err
	}
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
