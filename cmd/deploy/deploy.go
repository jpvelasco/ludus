package deploy

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	region       string
	instanceType string
	fleetName    string
	useCfn       bool
)

// Cmd is the top-level deploy command group.
var Cmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the container to AWS GameLift",
	Long: `Commands for deploying the containerized Lyra server to
AWS GameLift Containers. Supports both CloudFormation-based
deployment and direct API deployment.`,
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
  - CodeBuild project for CI/CD
  - IAM roles and policies
  - Optional: Cognito + API Gateway + Lambda for client auth`,
	RunE: runStack,
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Create a test game session",
	Long:  `Creates a game session on the deployed fleet for testing client connections.`,
	RunE:  runSession,
}

func init() {
	Cmd.PersistentFlags().StringVar(&region, "region", "us-east-1", "AWS region")
	Cmd.PersistentFlags().StringVar(&instanceType, "instance-type", "c6i.large", "EC2 instance type for the fleet")
	Cmd.PersistentFlags().StringVar(&fleetName, "fleet-name", "ludus-lyra-fleet", "name for the GameLift fleet")

	stackCmd.Flags().BoolVar(&useCfn, "with-auth", false, "include Cognito + API Gateway authentication stack")

	Cmd.AddCommand(fleetCmd)
	Cmd.AddCommand(stackCmd)
	Cmd.AddCommand(sessionCmd)
}

func runFleet(cmd *cobra.Command, args []string) error {
	fmt.Println("Fleet deployment not yet implemented.")
	fmt.Println()
	fmt.Println("This will:")
	fmt.Println("  1. Create container group definition from ECR image")
	fmt.Println("  2. Wait for image snapshot (COPYING -> READY)")
	fmt.Println("  3. Create IAM role with GameLiftContainerFleetPolicy")
	fmt.Println("  4. Create container fleet")
	fmt.Printf("  5. Region: %s, Instance: %s\n", region, instanceType)
	return nil
}

func runStack(cmd *cobra.Command, args []string) error {
	fmt.Println("CloudFormation stack deployment not yet implemented.")
	fmt.Printf("With auth stack: %t\n", useCfn)
	return nil
}

func runSession(cmd *cobra.Command, args []string) error {
	fmt.Println("Game session creation not yet implemented.")
	fmt.Println("This will create a test game session on the deployed fleet.")
	return nil
}
