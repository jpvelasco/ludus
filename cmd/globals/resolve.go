package globals

import (
	"context"
	"fmt"

	"github.com/jpvelasco/ludus/internal/anywhere"
	"github.com/jpvelasco/ludus/internal/awsenv"
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

// ResolveTargetFn is the function ResolveTarget delegates to.
// Tests may swap it; must restore via t.Cleanup.
var ResolveTargetFn = resolveTargetImpl

// ResolveTarget creates the appropriate deploy.Target based on config.
// The targetOverride parameter allows CLI flags to override the config value.
func ResolveTarget(ctx context.Context, cfg *config.Config, targetOverride string) (deploy.Target, error) {
	return ResolveTargetFn(ctx, cfg, targetOverride)
}

// resolveTargetImpl is the default implementation of ResolveTarget.
func resolveTargetImpl(ctx context.Context, cfg *config.Config, targetOverride string) (deploy.Target, error) {
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

// ResolveSessionTarget resolves the deploy target for session operations.
// Resolution order: the target recorded in deploy state (the most recent
// deployment, which reflects what actually runs), then the configured target
// when no deployment is recorded. A state file that cannot be read or parsed
// surfaces as a contextual error instead of a panic, because guessing the
// configured default could target the wrong deployment.
// All target resolution goes through ResolveTargetFn, so test swaps affect
// this too.
func ResolveSessionTarget(ctx context.Context, cfg *config.Config) (deploy.Target, error) {
	st, err := state.Load()
	if err != nil {
		return nil, fmt.Errorf("loading deploy state: %w", err)
	}
	if st.Deploy == nil || st.Deploy.TargetName == "" {
		return ResolveTarget(ctx, cfg, "")
	}
	target, err := ResolveTarget(ctx, cfg, st.Deploy.TargetName)
	if err != nil {
		return nil, fmt.Errorf("could not resolve deploy target from state (%q): %w", st.Deploy.TargetName, err)
	}
	return target, nil
}

func resolveGameLift(ctx context.Context, cfg *config.Config) (deploy.Target, error) {
	// Auto-default instance type based on server architecture
	if resolved, switched := pricing.AutoSwitch(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch()); switched {
		fmt.Printf("Note: Switching instance type from %s to %s to match %s server architecture\n",
			cfg.GameLift.InstanceType, resolved, cfg.Game.ResolvedArch())
		cfg.GameLift.InstanceType = resolved
	}

	env, err := awsenv.NewResolver(DryRun).Resolve(ctx, cfg, awsenv.Requirements{Account: true, Region: true})
	if err != nil {
		return nil, err
	}
	imageURI, err := awsenv.ImageURI(env, cfg.AWS.ECRRepository, cfg.Container.Tag)
	if err != nil {
		return nil, err
	}

	deployer := gamelift.NewDeployer(gamelift.DeployOptions{
		Region:                env.Region,
		ImageURI:              imageURI,
		FleetName:             cfg.GameLift.FleetName,
		InstanceType:          cfg.GameLift.InstanceType,
		ContainerGroupName:    cfg.GameLift.ContainerGroupName,
		ServerPort:            cfg.Container.ServerPort,
		MaxConcurrentSessions: cfg.GameLift.MaxConcurrentSessions,
		Tags:                  tags.Build(cfg),
	}, env.AWSConfig)

	return gamelift.NewTargetAdapter(deployer), nil
}

// recordedStackName returns the stack name recorded by the most recent stack
// deployment. Session, status, and destroy resolution operate on the existing
// deployment, whose stack can carry a custom name from --stack-name that the
// ludus-<fleet-name> default would miss. Returns an error when the state file
// cannot be read — falling back to the default name could silently target a
// different stack.
func recordedStackName() (string, error) {
	st, err := state.Load()
	if err != nil {
		return "", fmt.Errorf("reading deploy state for stack name: %w", err)
	}
	if st.Deploy == nil || st.Deploy.TargetName != "stack" || st.Fleet == nil {
		return "", nil
	}
	return st.Fleet.StackName, nil
}

// BuildStackOptions assembles the CloudFormation deployer options from the
// effective config and resolved runtime inputs. The CLI stack deploy, the MCP
// deploy tool, and target resolution share this one assembly so the wiring
// cannot drift between entry points.
func BuildStackOptions(cfg *config.Config, env awsenv.Env, imageURI, stackName, fleetName string) stack.StackOptions {
	return stack.StackOptions{
		StackName:             stackName,
		Region:                env.Region,
		ImageURI:              imageURI,
		FleetName:             fleetName,
		InstanceType:          cfg.GameLift.InstanceType,
		ContainerGroupName:    cfg.GameLift.ContainerGroupName,
		ServerPort:            cfg.Container.ServerPort,
		ServerSDKVersion:      cfg.EC2Fleet.ResolvedServerSDKVersion(),
		MaxConcurrentSessions: cfg.GameLift.MaxConcurrentSessions,
		Tags:                  tags.Build(cfg),
	}
}

func resolveStack(ctx context.Context, cfg *config.Config) (deploy.Target, error) {
	// Auto-default instance type based on server architecture
	if resolved, switched := pricing.AutoSwitch(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch()); switched {
		fmt.Printf("Note: Switching instance type from %s to %s to match %s server architecture\n",
			cfg.GameLift.InstanceType, resolved, cfg.Game.ResolvedArch())
		cfg.GameLift.InstanceType = resolved
	}

	env, err := awsenv.NewResolver(DryRun).Resolve(ctx, cfg, awsenv.Requirements{Account: true, Region: true})
	if err != nil {
		return nil, err
	}
	imageURI, err := awsenv.ImageURI(env, cfg.AWS.ECRRepository, cfg.Container.Tag)
	if err != nil {
		return nil, err
	}

	stackName, err := recordedStackName()
	if err != nil {
		return nil, err
	}
	if stackName == "" {
		stackName = fmt.Sprintf("ludus-%s", cfg.GameLift.FleetName)
	}

	deployer := stack.NewStackDeployer(BuildStackOptions(cfg, env, imageURI, stackName, cfg.GameLift.FleetName), env.AWSConfig)

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
	env, err := awsenv.NewResolver(DryRun).Resolve(ctx, cfg, awsenv.Requirements{Region: true})
	if err != nil {
		return nil, err
	}
	awsCfg := env.AWSConfig

	// Resolve server build directory
	serverBuildDir := config.ResolveServerBuildDir(cfg)
	if serverBuildDir == "" {
		return nil, fmt.Errorf("could not determine server build directory; set game.projectPath in ludus.yaml")
	}

	r := NewRunner()

	deployer := anywhere.NewDeployer(anywhere.DeployOptions{
		Region:          env.Region,
		FleetName:       cfg.GameLift.FleetName,
		LocationName:    cfg.Anywhere.LocationName,
		IPAddress:       cfg.Anywhere.IPAddress,
		ServerPort:      cfg.Container.ServerPort,
		Tags:            tags.Build(cfg),
		ServerBuildDir:  serverBuildDir,
		ProjectName:     cfg.Game.ProjectName,
		PackagedDirName: cfg.Game.ResolvedPackagedDirName(),
		ServerTarget:    cfg.Game.ResolvedServerTarget(),
		ServerMap:       cfg.Game.ServerMap,
		AWSProfile:      cfg.Anywhere.AWSProfile,
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

	env, err := awsenv.NewResolver(DryRun).Resolve(ctx, cfg, awsenv.Requirements{Region: true})
	if err != nil {
		return nil, err
	}
	awsCfg := env.AWSConfig

	r := NewRunner()

	deployer := ec2fleet.NewDeployer(ec2fleet.DeployOptions{
		Region:                env.Region,
		FleetName:             cfg.GameLift.FleetName,
		InstanceType:          cfg.GameLift.InstanceType,
		ServerPort:            cfg.Container.ServerPort,
		MaxConcurrentSessions: cfg.GameLift.MaxConcurrentSessions,
		S3Bucket:              cfg.EC2Fleet.S3Bucket,
		ProjectName:           cfg.Game.ProjectName,
		PackagedDirName:       cfg.Game.ResolvedPackagedDirName(),
		ServerTarget:          cfg.Game.ResolvedServerTarget(),
		ServerMap:             cfg.Game.ServerMap,
		Arch:                  cfg.Game.ResolvedArch(),
		AllowedCIDR:           config.ResolvedAllowedCIDR(cfg.EC2Fleet.AllowedCIDR),
		Tags:                  tags.Build(cfg),
	}, awsCfg, r)

	return ec2fleet.NewTargetAdapter(deployer), nil
}
