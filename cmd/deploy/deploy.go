package deploy

import (
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/gamelift"
	"github.com/spf13/cobra"
)

var (
	region       string
	instanceType string
	fleetName    string
)

// Cmd is the top-level deploy command group.
var Cmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the container to AWS GameLift",
	Long: `Commands for deploying the containerized Lyra server to
AWS GameLift Containers.`,
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
	Short: "Tear down all Ludus-managed AWS resources",
	Long: `Destroys all AWS resources created by Ludus in reverse order:

  1. Deletes the GameLift container fleet (waits for deletion)
  2. Deletes the container group definition
  3. Detaches policies and deletes the IAM role

Resources that don't exist are skipped gracefully.`,
	RunE: runDestroy,
}

func init() {
	Cmd.PersistentFlags().StringVar(&region, "region", "", "AWS region (default: from ludus.yaml)")
	Cmd.PersistentFlags().StringVar(&instanceType, "instance-type", "", "EC2 instance type (default: from ludus.yaml)")
	Cmd.PersistentFlags().StringVar(&fleetName, "fleet-name", "", "GameLift fleet name (default: from ludus.yaml)")

	Cmd.AddCommand(fleetCmd)
	Cmd.AddCommand(stackCmd)
	Cmd.AddCommand(sessionCmd)
	Cmd.AddCommand(destroyCmd)
}

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
	status, err := deployer.CreateFleet(cmd.Context(), cgdARN)
	if err != nil {
		return err
	}

	fmt.Printf("\nFleet deployed: %s (status: %s)\n", status.FleetID, status.Status)
	return nil
}

func runStack(cmd *cobra.Command, args []string) error {
	fmt.Println("CloudFormation stack deployment not yet implemented.")
	return nil
}

func runSession(cmd *cobra.Command, args []string) error {
	deployer, err := makeDeployer(cmd)
	if err != nil {
		return err
	}

	// Find the active fleet
	fleetStatus, err := deployer.GetFleetStatus(cmd.Context())
	if err != nil {
		return fmt.Errorf("finding fleet: %w", err)
	}

	fmt.Printf("Creating game session on fleet %s...\n", fleetStatus.FleetID)
	sessionID, err := deployer.CreateGameSession(cmd.Context(), fleetStatus.FleetID, 8)
	if err != nil {
		return err
	}

	fmt.Printf("Game session created: %s\n", sessionID)
	return nil
}

func runDestroy(cmd *cobra.Command, args []string) error {
	deployer, err := makeDeployer(cmd)
	if err != nil {
		return err
	}

	fmt.Println("Destroying all Ludus-managed AWS resources...")
	if err := deployer.Destroy(cmd.Context()); err != nil {
		return err
	}

	fmt.Println("\nAll resources destroyed.")
	return nil
}
