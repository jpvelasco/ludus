package globals

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/devrecon/ludus/internal/anywhere"
	"github.com/devrecon/ludus/internal/binary"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/ec2fleet"
	"github.com/devrecon/ludus/internal/gamelift"
	"github.com/devrecon/ludus/internal/runner"
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
	case "anywhere":
		return resolveAnywhere(ctx, cfg)
	case "ec2":
		return resolveEC2Fleet(ctx, cfg)
	default:
		return nil, fmt.Errorf("unknown deploy target %q (supported: gamelift, stack, binary, anywhere, ec2)", target)
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

func resolveAnywhere(ctx context.Context, cfg *config.Config) (deploy.Target, error) {
	awsCfg, err := gamelift.LoadAWSConfig(ctx, cfg.AWS.Region)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	// Resolve server build directory
	serverBuildDir := resolveServerBuildDir(cfg)
	if serverBuildDir == "" {
		return nil, fmt.Errorf("could not determine server build directory; set game.projectPath in ludus.yaml")
	}

	r := runner.NewRunner(Verbose, DryRun)

	deployer := anywhere.NewDeployer(anywhere.DeployOptions{
		Region:         cfg.AWS.Region,
		FleetName:      cfg.GameLift.FleetName,
		LocationName:   cfg.Anywhere.LocationName,
		IPAddress:      cfg.Anywhere.IPAddress,
		ServerPort:     cfg.Container.ServerPort,
		Tags:           tags.Build(cfg),
		ServerBuildDir: serverBuildDir,
		ProjectName:    cfg.Game.ProjectName,
		ServerTarget:   cfg.Game.ResolvedServerTarget(),
		ServerMap:      cfg.Game.ServerMap,
		AWSProfile:     cfg.Anywhere.AWSProfile,
	}, awsCfg, r)

	return anywhere.NewTargetAdapter(deployer), nil
}

func resolveEC2Fleet(ctx context.Context, cfg *config.Config) (deploy.Target, error) {
	awsCfg, err := gamelift.LoadAWSConfig(ctx, cfg.AWS.Region)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	r := runner.NewRunner(Verbose, DryRun)

	deployer := ec2fleet.NewDeployer(ec2fleet.DeployOptions{
		Region:       cfg.AWS.Region,
		FleetName:    cfg.GameLift.FleetName,
		InstanceType: cfg.GameLift.InstanceType,
		ServerPort:   cfg.Container.ServerPort,
		S3Bucket:     cfg.EC2Fleet.S3Bucket,
		ProjectName:  cfg.Game.ProjectName,
		ServerTarget: cfg.Game.ResolvedServerTarget(),
		ServerMap:    cfg.Game.ServerMap,
		Arch:         cfg.Game.ResolvedArch(),
		Tags:         tags.Build(cfg),
	}, awsCfg, r)

	return ec2fleet.NewTargetAdapter(deployer), nil
}

// resolveServerBuildDir determines the server build directory from config.
func resolveServerBuildDir(cfg *config.Config) string {
	platformDir := config.ServerPlatformDir(cfg.Game.ResolvedArch())
	if cfg.Game.ProjectPath != "" {
		return filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "PackagedServer", platformDir)
	}
	if cfg.Engine.SourcePath != "" && cfg.Game.ProjectName == "Lyra" {
		return filepath.Join(cfg.Engine.SourcePath, "Samples", "Games", "Lyra", "PackagedServer", platformDir)
	}
	return ""
}
