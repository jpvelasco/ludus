package container

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	tag       string
	noCache   bool
	pushToECR bool
)

// Cmd is the top-level container command group.
var Cmd = &cobra.Command{
	Use:   "container",
	Short: "Containerize the Lyra dedicated server",
	Long: `Commands for building and managing the Docker container image
for the Lyra dedicated server. Uses the GameLift Containers
Starter Kit patterns (Amazon Linux 2023 base, non-root user,
Go SDK wrapper).`,
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the Docker container image",
	Long: `Builds a Docker image containing:

  - Amazon Linux 2023 base image
  - Lyra dedicated server binary (Linux)
  - GameLift Go SDK wrapper (handles SDK lifecycle)
  - Wrapper script for process orchestration
  - Non-root user (required for Unreal servers)`,
	RunE: runBuild,
}

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push the container image to Amazon ECR",
	Long: `Authenticates with Amazon ECR and pushes the built container
image to the configured repository.`,
	RunE: runPush,
}

func init() {
	buildCmd.Flags().StringVarP(&tag, "tag", "t", "latest", "image tag")
	buildCmd.Flags().BoolVar(&noCache, "no-cache", false, "build without Docker cache")

	pushCmd.Flags().BoolVar(&pushToECR, "create-repo", false, "create the ECR repository if it doesn't exist")

	Cmd.AddCommand(buildCmd)
	Cmd.AddCommand(pushCmd)
}

func runBuild(cmd *cobra.Command, args []string) error {
	fmt.Println("Container build not yet implemented.")
	fmt.Println()
	fmt.Println("This will:")
	fmt.Println("  1. Generate Dockerfile from template")
	fmt.Println("  2. Copy Lyra server build into Docker context")
	fmt.Println("  3. Build Go SDK wrapper")
	fmt.Println("  4. Build Docker image with non-root user")
	fmt.Printf("  5. Tag: %s, No-cache: %t\n", tag, noCache)
	return nil
}

func runPush(cmd *cobra.Command, args []string) error {
	fmt.Println("Container push not yet implemented.")
	fmt.Println()
	fmt.Println("This will:")
	fmt.Println("  1. Authenticate with Amazon ECR")
	fmt.Println("  2. Tag image for ECR repository")
	fmt.Println("  3. Push image to ECR")
	return nil
}
