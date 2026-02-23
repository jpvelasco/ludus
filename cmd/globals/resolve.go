package globals

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/internal/binary"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/gamelift"
	"github.com/devrecon/ludus/internal/stack"
	"github.com/devrecon/ludus/internal/tags"
)

// ResolveTarget creates the appropriate deploy.Target based on config.
// The targetOverride parameter allows CLI flags to override the config value.
func ResolveTarget(ctx context.Context, cfg *config.Config, targetOverride string) (deploy.Target, error) {
	target := targetOverride
	if target == "" {
		target = cfg.Deploy.Target
	}

	switch target {
	case "gamelift", "":
		return resolveGameLift(ctx, cfg)
	case "stack":
		return resolveStack(ctx, cfg)
	case "binary":
		return resolveBinary(cfg)
	default:
		return nil, fmt.Errorf("unknown deploy target %q (supported: gamelift, stack, binary)", target)
	}
}

func resolveGameLift(ctx context.Context, cfg *config.Config) (deploy.Target, error) {
	awsCfg, err := gamelift.LoadAWSConfig(ctx, cfg.AWS.Region)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	imageURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
		cfg.AWS.AccountID, cfg.AWS.Region, cfg.AWS.ECRRepository, cfg.Container.Tag)

	deployer := gamelift.NewDeployer(gamelift.DeployOptions{
		Region:             cfg.AWS.Region,
		ImageURI:           imageURI,
		FleetName:          cfg.GameLift.FleetName,
		InstanceType:       cfg.GameLift.InstanceType,
		ContainerGroupName: cfg.GameLift.ContainerGroupName,
		ServerPort:         cfg.Container.ServerPort,
		Tags:               tags.Build(cfg),
	}, awsCfg)

	return gamelift.NewTargetAdapter(deployer), nil
}

func resolveStack(ctx context.Context, cfg *config.Config) (deploy.Target, error) {
	awsCfg, err := gamelift.LoadAWSConfig(ctx, cfg.AWS.Region)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	imageURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
		cfg.AWS.AccountID, cfg.AWS.Region, cfg.AWS.ECRRepository, cfg.Container.Tag)

	deployer := stack.NewStackDeployer(stack.StackOptions{
		StackName:          fmt.Sprintf("ludus-%s", cfg.GameLift.FleetName),
		Region:             cfg.AWS.Region,
		ImageURI:           imageURI,
		FleetName:          cfg.GameLift.FleetName,
		InstanceType:       cfg.GameLift.InstanceType,
		ContainerGroupName: cfg.GameLift.ContainerGroupName,
		ServerPort:         cfg.Container.ServerPort,
		ServerSDKVersion:   "5.4.0",
		Tags:               tags.Build(cfg),
	}, awsCfg)

	return stack.NewTargetAdapter(deployer), nil
}

func resolveBinary(cfg *config.Config) (deploy.Target, error) {
	outputDir := cfg.Deploy.OutputDir
	if outputDir == "" {
		outputDir = "./dist/server"
	}
	return binary.NewExporter(outputDir), nil
}
