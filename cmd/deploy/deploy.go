package deploy

import (
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/gamelift"
	"github.com/devrecon/ludus/internal/state"
	"github.com/spf13/cobra"
)

var (
	region       string
	instanceType string
	fleetName    string
	targetFlag   string
)

// Cmd is the top-level deploy command group.
var Cmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the game server to a target",
	Long: `Commands for deploying the game server to a deployment target.

Supported targets: gamelift (default), binary.
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
	Short: "Deploy the full backend stack via CloudFormation",
	Long: `Deploys a CloudFormation stack that provisions:

  - GameLift container fleet
  - ECR repository
  - IAM roles and policies`,
	RunE: runStack,
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Create a test game session",
	Long:  `Creates a game session on the deployed fleet for testing client connections.`,
	RunE:  runSession,
}

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Tear down all deployed resources",
	Long: `Destroys all resources created by Ludus for the active deployment target.

For GameLift: deletes fleet, container group definition, and IAM role.
For binary: removes the output directory.

Resources that don't exist are skipped gracefully.`,
	RunE: runDestroy,
}

func init() {
	Cmd.PersistentFlags().StringVar(&targetFlag, "target", "", "deployment target: gamelift, binary (default: from ludus.yaml)")
	Cmd.PersistentFlags().StringVar(&region, "region", "", "AWS region (default: from ludus.yaml)")
	Cmd.PersistentFlags().StringVar(&instanceType, "instance-type", "", "EC2 instance type (default: from ludus.yaml)")
	Cmd.PersistentFlags().StringVar(&fleetName, "fleet-name", "", "GameLift fleet name (default: from ludus.yaml)")

	Cmd.AddCommand(fleetCmd)
	Cmd.AddCommand(stackCmd)
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
	fmt.Println("CloudFormation stack deployment not yet implemented.")
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
