package deploy

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/gamelift"
	"github.com/devrecon/ludus/internal/stack"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/tags"
	"github.com/spf13/cobra"
)

var (
	region       string
	instanceType string
	fleetName    string
	targetFlag   string
	stackName    string
	anywhereIP   string
	ec2Arch      string
)

// Cmd is the top-level deploy command group.
var Cmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the game server to a target",
	Long: `Commands for deploying the game server to a deployment target.

Supported targets: gamelift (default), stack, binary, anywhere, ec2.
Use --target to override the target from ludus.yaml.`,
}

var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Create or update a GameLift container fleet",
	Long: `Deploys the container to GameLift by:

  1. Creating a container group definition
  2. Waiting for the image to be snapshotted (COPYING -> READY)
  3. Creating/updating the container fleet
  4. Configuring inbound permissions (UDP 7777)`,
	RunE: runFleet,
}

var stackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Deploy via CloudFormation stack",
	Long: `Deploys a CloudFormation stack that atomically provisions:

  - IAM role for GameLift container fleet
  - Container group definition
  - Container fleet with inbound permissions

The stack provides atomic deployments with automatic rollback on failure.
Use --stack-name to override the default stack name (ludus-<fleet-name>).`,
	RunE: runStack,
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Create a test game session",
	Long:  `Creates a game session on the deployed fleet for testing client connections.`,
	RunE:  runSession,
}

var anywhereCmd = &cobra.Command{
	Use:   "anywhere",
	Short: "Deploy a local Anywhere fleet and launch the game server",
	Long: `Creates a GameLift Anywhere fleet, registers this machine as a compute,
and launches the game server via the GameLift Game Server Wrapper.

The server runs locally but GameLift manages sessions, matchmaking, and
player validation. Fleet creation takes seconds, not minutes.

Use --ip to override the auto-detected local IP address.`,
	RunE: runAnywhere,
}

var ec2Cmd = &cobra.Command{
	Use:   "ec2",
	Short: "Deploy a GameLift Managed EC2 fleet",
	Long: `Deploys the server build to a GameLift Managed EC2 fleet by:

  1. Zipping the server build with the Game Server Wrapper
  2. Uploading to S3
  3. Creating a GameLift Build
  4. Creating an EC2 fleet with runtime configuration
  5. Waiting for fleet to become ACTIVE

No Docker or containers required — GameLift runs the server binary directly on EC2.`,
	RunE: runEC2,
}

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Tear down all deployed resources",
	Long: `Destroys all resources created by Ludus for the active deployment target.

For GameLift: deletes fleet, container group definition, and IAM role.
For stack: deletes the CloudFormation stack (all resources removed atomically).
For binary: removes the output directory.
For anywhere: stops server, deregisters compute, deletes fleet and location.
For ec2: deletes fleet, build, S3 object, and IAM role.

Resources that don't exist are skipped gracefully.`,
	RunE: runDestroy,
}

func init() {
	Cmd.PersistentFlags().StringVar(&targetFlag, "target", "", "deployment target: gamelift, stack, binary, anywhere, ec2 (default: from ludus.yaml)")
	Cmd.PersistentFlags().StringVar(&region, "region", "", "AWS region (default: from ludus.yaml)")
	Cmd.PersistentFlags().StringVar(&instanceType, "instance-type", "", "EC2 instance type (default: from ludus.yaml)")
	Cmd.PersistentFlags().StringVar(&fleetName, "fleet-name", "", "GameLift fleet name (default: from ludus.yaml)")

	stackCmd.Flags().StringVar(&stackName, "stack-name", "", "CloudFormation stack name (default: ludus-<fleet-name>)")
	anywhereCmd.Flags().StringVar(&anywhereIP, "ip", "", "local IP address override (default: auto-detect)")
	ec2Cmd.Flags().StringVar(&ec2Arch, "arch", "", `target CPU architecture: amd64, arm64 (default: from ludus.yaml)`)

	Cmd.AddCommand(fleetCmd)
	Cmd.AddCommand(stackCmd)
	Cmd.AddCommand(anywhereCmd)
	Cmd.AddCommand(ec2Cmd)
	Cmd.AddCommand(sessionCmd)
	Cmd.AddCommand(destroyCmd)
}

// makeDeployer creates a GameLift deployer with flag overrides applied.
// Used by GameLift-specific commands (fleet, session) that need direct Deployer access.
func makeDeployer(cmd *cobra.Command) (*gamelift.Deployer, error) {
	cfg := globals.Cfg

	r := region
	if r == "" {
		r = cfg.AWS.Region
	}
	it := instanceType
	if it == "" {
		it = cfg.GameLift.InstanceType
	}
	fn := fleetName
	if fn == "" {
		fn = cfg.GameLift.FleetName
	}

	imageURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
		cfg.AWS.AccountID, r, cfg.AWS.ECRRepository, cfg.Container.Tag)

	awsCfg, err := gamelift.LoadAWSConfig(cmd.Context(), r)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	return gamelift.NewDeployer(gamelift.DeployOptions{
		Region:             r,
		ImageURI:           imageURI,
		FleetName:          fn,
		InstanceType:       it,
		ContainerGroupName: cfg.GameLift.ContainerGroupName,
		ServerPort:         cfg.Container.ServerPort,
		Tags:               tags.Build(cfg),
	}, awsCfg), nil
}

// resolveTarget resolves a deploy.Target, applying --target flag override and
// flag overrides for GameLift-specific flags (--region, --instance-type, --fleet-name).
func resolveTarget(cmd *cobra.Command) (deploy.Target, error) {
	cfg := globals.Cfg

	// Apply flag overrides to config before resolving
	if region != "" {
		cfg.AWS.Region = region
	}
	if instanceType != "" {
		cfg.GameLift.InstanceType = instanceType
	}
	if fleetName != "" {
		cfg.GameLift.FleetName = fleetName
	}

	return globals.ResolveTarget(cmd.Context(), cfg, targetFlag)
}

func runFleet(cmd *cobra.Command, args []string) error {
	deployer, err := makeDeployer(cmd)
	if err != nil {
		return err
	}

	fmt.Println("Creating container group definition...")
	cgdARN, err := deployer.CreateContainerGroupDefinition(cmd.Context())
	if err != nil {
		return err
	}
	fmt.Printf("Container group definition ready: %s\n\n", cgdARN)

	fmt.Println("Creating container fleet...")
	fleetStatus, err := deployer.CreateFleet(cmd.Context(), cgdARN)
	if err != nil {
		return err
	}

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   fleetStatus.FleetID,
		Status:    fleetStatus.Status,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}

	fmt.Printf("\nFleet deployed: %s (status: %s)\n", fleetStatus.FleetID, fleetStatus.Status)
	return nil
}

func runStack(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	// Apply flag overrides
	if region != "" {
		cfg.AWS.Region = region
	}
	if instanceType != "" {
		cfg.GameLift.InstanceType = instanceType
	}
	fn := fleetName
	if fn == "" {
		fn = cfg.GameLift.FleetName
	}

	sn := stackName
	if sn == "" {
		sn = fmt.Sprintf("ludus-%s", fn)
	}

	r := cfg.AWS.Region
	imageURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
		cfg.AWS.AccountID, r, cfg.AWS.ECRRepository, cfg.Container.Tag)

	awsCfg, err := gamelift.LoadAWSConfig(cmd.Context(), r)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	start := time.Now()
	deployer := stack.NewStackDeployer(stack.StackOptions{
		StackName:          sn,
		Region:             r,
		ImageURI:           imageURI,
		FleetName:          fn,
		InstanceType:       cfg.GameLift.InstanceType,
		ContainerGroupName: cfg.GameLift.ContainerGroupName,
		ServerPort:         cfg.Container.ServerPort,
		ServerSDKVersion:   "5.4.0",
		Tags:               tags.Build(cfg),
	}, awsCfg)

	result, err := deployer.Deploy(cmd.Context())
	if err != nil {
		return err
	}

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   result.FleetID,
		StackName: result.StackName,
		Status:    result.Status,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}

	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "stack",
		Status:     result.Status,
		Detail:     fmt.Sprintf("stack %s, fleet %s", result.StackName, result.FleetID),
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write deploy state: %v\n", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("\nStack deployed: %s (status: %s)\n", result.StackName, result.Status)
	if result.FleetID != "" {
		fmt.Printf("Fleet ID: %s\n", result.FleetID)
	}
	fmt.Printf("Duration: %s\n", elapsed.Round(time.Second))
	return nil
}

func runSession(cmd *cobra.Command, args []string) error {
	target, err := resolveTarget(cmd)
	if err != nil {
		return err
	}

	sm, ok := target.(deploy.SessionManager)
	if !ok {
		return fmt.Errorf("target %q does not support game sessions", target.Name())
	}

	fmt.Println("Creating game session...")
	info, err := sm.CreateSession(cmd.Context(), 8)
	if err != nil {
		return err
	}

	fmt.Printf("Game session created: %s\n", info.SessionID)
	return nil
}

func runAnywhere(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	// Apply flag overrides
	if region != "" {
		cfg.AWS.Region = region
	}
	if fleetName != "" {
		cfg.GameLift.FleetName = fleetName
	}
	if anywhereIP != "" {
		cfg.Anywhere.IPAddress = anywhereIP
	}

	target, err := globals.ResolveTarget(cmd.Context(), cfg, "anywhere")
	if err != nil {
		return err
	}

	result, err := target.Deploy(cmd.Context(), deploy.DeployInput{
		ServerPort: cfg.Container.ServerPort,
	})
	if err != nil {
		return err
	}

	fmt.Printf("\nAnywhere deployment ready: %s\n", result.Detail)
	fmt.Println("Run 'ludus deploy session' to create a game session.")
	return nil
}

func runEC2(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	// Apply flag overrides
	if region != "" {
		cfg.AWS.Region = region
	}
	if instanceType != "" {
		cfg.GameLift.InstanceType = instanceType
	}
	if fleetName != "" {
		cfg.GameLift.FleetName = fleetName
	}
	if ec2Arch != "" {
		cfg.Game.Arch = ec2Arch
	}

	target, err := globals.ResolveTarget(cmd.Context(), cfg, "ec2")
	if err != nil {
		return err
	}

	serverBuildDir := resolveServerBuildDirFromCfg(cfg)
	if serverBuildDir == "" {
		return fmt.Errorf("could not determine server build directory; set game.projectPath in ludus.yaml")
	}

	start := time.Now()
	result, err := target.Deploy(cmd.Context(), deploy.DeployInput{
		ServerBuildDir: serverBuildDir,
		ServerPort:     cfg.Container.ServerPort,
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start)
	fmt.Printf("\nEC2 fleet deployed: %s\n", result.Detail)
	fmt.Printf("Duration: %s\n", elapsed.Round(time.Second))
	fmt.Println("Run 'ludus deploy session' to create a game session.")
	return nil
}

// resolveServerBuildDirFromCfg determines the server build directory from config.
func resolveServerBuildDirFromCfg(cfg *config.Config) string {
	platformDir := config.ServerPlatformDir(cfg.Game.ResolvedArch())
	if cfg.Game.ProjectPath != "" {
		return filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "PackagedServer", platformDir)
	}
	if cfg.Engine.SourcePath != "" && cfg.Game.ProjectName == "Lyra" {
		return filepath.Join(cfg.Engine.SourcePath, "Samples", "Games", "Lyra", "PackagedServer", platformDir)
	}
	return ""
}

func runDestroy(cmd *cobra.Command, args []string) error {
	target, err := resolveTarget(cmd)
	if err != nil {
		return err
	}

	fmt.Printf("Destroying %s resources...\n", target.Name())
	if err := target.Destroy(cmd.Context()); err != nil {
		return err
	}

	fmt.Printf("\nAll %s resources destroyed.\n", target.Name())
	return nil
}
