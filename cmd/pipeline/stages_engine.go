package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
	engBuilder "github.com/jpvelasco/ludus/internal/engine"
	"github.com/jpvelasco/ludus/internal/state"
)

func (p *pipelineCtx) stageEngineBuild(ctx context.Context) error {
	if p.checkCacheSkip(cache.StageEngine, p.engineHash, "Engine build") {
		return nil
	}
	if err := p.dispatchEngineBuild(ctx); err != nil {
		return err
	}
	p.recordCache(cache.StageEngine, p.engineHash)
	return nil
}

func (p *pipelineCtx) dispatchEngineBuild(ctx context.Context) error {
	switch {
	case dockerbuild.IsContainerBackend(p.containerBackend):
		return p.buildEngineContainer(ctx)
	case dockerbuild.IsWSL2Backend(p.containerBackend):
		result, err := p.buildEngineWSL2(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("    Engine built in WSL2 in %.0fs: %s\n", result.Duration, result.EnginePath)
	default:
		result, err := p.buildEngineNative(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("    Engine built in %.0fs\n", result.Duration)
	}
	return nil
}

func (p *pipelineCtx) buildEngineContainer(ctx context.Context) error {
	if p.cfg.Engine.DockerImage != "" {
		fmt.Printf("    Using pre-built engine image: %s\n", p.cfg.Engine.DockerImage)
		return nil
	}

	builder := dockerbuild.NewEngineImageBuilder(dockerbuild.EngineImageOptions{
		SourcePath: p.cfg.Engine.SourcePath,
		Version:    p.engineVersion,
		MaxJobs:    p.cfg.Engine.MaxJobs,
		ImageName:  p.engineImageName(),
		BaseImage:  p.cfg.Engine.DockerBaseImage,
		Runtime:    p.containerBackend,
	}, p.r)
	result, err := builder.Build(ctx)
	if err != nil {
		return err
	}

	p.saveEngineImageState(result.ImageTag)
	cli := dockerbuild.ContainerCLI(p.containerBackend)
	fmt.Printf("    Engine %s image built in %.0fs: %s\n", cli, result.Duration, result.ImageTag)
	return nil
}

func (p *pipelineCtx) engineImageName() string {
	if p.cfg.Engine.DockerImageName != "" {
		return p.cfg.Engine.DockerImageName
	}
	return "ludus-engine"
}

func (p *pipelineCtx) saveEngineImageState(imageTag string) {
	if err := state.UpdateEngineImage(&state.EngineImageState{
		ImageTag: imageTag,
		BuiltAt:  time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("    Warning: failed to write state: %v\n", err)
	}
}

func (p *pipelineCtx) buildEngineNative(ctx context.Context) (*engBuilder.BuildResult, error) {
	builder := engBuilder.NewBuilder(engBuilder.BuildOptions{
		SourcePath: p.cfg.Engine.SourcePath,
		MaxJobs:    p.cfg.Engine.MaxJobs,
		Verbose:    globals.Verbose,
	}, p.r)
	return builder.Build(ctx)
}
