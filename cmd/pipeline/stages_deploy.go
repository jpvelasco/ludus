package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/awsenv"
	"github.com/jpvelasco/ludus/internal/cache"
	ctrBuilder "github.com/jpvelasco/ludus/internal/container"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/dflint"
	"github.com/jpvelasco/ludus/internal/ecr"
	"github.com/jpvelasco/ludus/internal/pricing"
	"github.com/jpvelasco/ludus/internal/state"
)

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

	imageURI, err := p.buildImageURI(ctx)
	if err != nil {
		return err
	}
	result, err := p.target.Deploy(ctx, deploy.DeployInput{
		ImageURI:       imageURI,
		ServerBuildDir: p.serverBuildDir,
		ServerPort:     p.cfg.Container.ServerPort,
	})
	if err != nil {
		return err
	}

	p.saveDeployState(result)
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

func (p *pipelineCtx) buildImageURI(ctx context.Context) (string, error) {
	env, err := awsenv.NewResolver(globals.DryRun).Resolve(ctx, p.cfg, awsenv.Requirements{Account: true, Region: true})
	if err != nil {
		return "", err
	}
	return awsenv.ImageURI(env, p.cfg.AWS.ECRRepository, p.cfg.Container.Tag)
}

func (p *pipelineCtx) saveDeployState(result *deploy.DeployResult) {
	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: result.TargetName,
		Status:     result.Status,
		Detail:     result.Detail,
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("    Warning: failed to write state: %v\n", err)
	}
}
