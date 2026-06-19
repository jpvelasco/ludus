package globals

import (
	"context"
	"fmt"

	"github.com/jpvelasco/ludus/internal/anywhere"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/binary"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/ec2fleet"
	"github.com/jpvelasco/ludus/internal/gamelift"
	"github.com/jpvelasco/ludus/internal/pricing"
	"github.com/jpvelasco/ludus/internal/stack"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/tags"
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

// ResolveSessionTarget resolves a deploy target that supports sessions.
// It first resolves the configured target; if that target doesn't implement
// deploy.SessionManager it falls back to the target stored in deploy state.
func ResolveSessionTarget(ctx context.Context, cfg *config.Config) (deploy.Target, error) {
	target, err := ResolveTarget(ctx, cfg, "")
	if err != nil {
		return nil, err
	}
	if _, ok := target.(deploy.SessionManager); ok {
		return target, nil
	}
	st, _ := state.Load()
	if st.Deploy == nil || st.Deploy.TargetName == "" {
		return target, nil
	}
	fallback, ferr := ResolveTarget(ctx, cfg, st.Deploy.TargetName)
	if ferr != nil {
		return nil, fmt.Errorf("could not resolve deploy target from state (%q): %w", st.Deploy.TargetName, ferr)
	}
	return fallback, nil
}

func resolveGameLift(ctx context.Context, cfg *config.Config) (deploy.Target, error) {
	// Auto-default instance type based on server architecture
	if resolved, switched := pricing.AutoSwitch(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch()); switched {
		fmt.Printf("Note: Switching instance type from %s to %s to match %s server architecture\n",
			cfg.GameLift.InstanceType, resolved, cfg.Game.ResolvedArch())
		cfg.GameLift.InstanceType = resolved
	}

	awsCfg, err := awsutil.LoadAWSConfig(ctx, cfg.AWS.Region)
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
	// Auto-default instance type based on server architecture
	if resolved, switched := pricing.AutoSwitch(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch()); switched {
		fmt.Printf("Note: Switching instance type from %s to %s to match %s server architecture\n",
			cfg.GameLift.InstanceType, resolved, cfg.Game.ResolvedArch())
		cfg.GameLift.InstanceType = resolved
	}

	awsCfg, err := awsutil.LoadAWSConfig(ctx, cfg.AWS.Region)
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
	awsCfg, err := awsutil.LoadAWSConfig(ctx, cfg.AWS.Region)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	// Resolve server build directory
	serverBuildDir := config.ResolveServerBuildDir(cfg)
	if serverBuildDir == "" {
		return nil, fmt.Errorf("could not determine server build directory; set game.projectPath in ludus.yaml")
	}

	r := NewRunner()

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
	// Auto-default instance type based on server architecture
	if resolved, switched := pricing.AutoSwitch(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch()); switched {
		fmt.Printf("Note: Switching instance type from %s to %s to match %s server architecture\n",
			cfg.GameLift.InstanceType, resolved, cfg.Game.ResolvedArch())
		cfg.GameLift.InstanceType = resolved
	}

	awsCfg, err := awsutil.LoadAWSConfig(ctx, cfg.AWS.Region)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	r := NewRunner()

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
