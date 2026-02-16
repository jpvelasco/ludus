package container

import (
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	ctrBuilder "github.com/devrecon/ludus/internal/container"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/spf13/cobra"
)

var (
	tag     string
	noCache bool
)

// Cmd is the top-level container command group.
var Cmd = &cobra.Command{
	Use:   "container",
	Short: "Containerize the Lyra dedicated server",
	Long: `Commands for building and managing the Docker container image
for the Lyra dedicated server. Uses the GameLift Containers
Starter Kit patterns (Amazon Linux 2023 base, non-root user).`,
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the Docker container image",
	Long: `Builds a Docker image containing:

  - Amazon Linux 2023 base image
  - Lyra dedicated server binary (Linux)
  - Non-root user (required for Unreal servers)
  - UDP port exposed for game traffic`,
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

	Cmd.AddCommand(buildCmd)
	Cmd.AddCommand(pushCmd)
}

func runBuild(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg
	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
		ServerBuildDir: cfg.Engine.SourcePath, // Will be overridden by pipeline; placeholder for standalone use
		ImageName:      cfg.Container.ImageName,
		Tag:            tag,
		ServerPort:     cfg.Container.ServerPort,
		NoCache:        noCache,
	}, r)

	fmt.Println("Building container image...")
	result, err := builder.Build(cmd.Context())
	if err != nil {
		return err
	}

	fmt.Printf("Container image built: %s (%.0fs)\n", result.ImageTag, result.Duration)
	return nil
}

func runPush(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg
	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
		ImageName:  cfg.Container.ImageName,
		Tag:        cfg.Container.Tag,
		ServerPort: cfg.Container.ServerPort,
	}, r)

	fmt.Println("Pushing container image to ECR...")
	return builder.Push(cmd.Context(), ctrBuilder.PushOptions{
		ECRRepository: cfg.AWS.ECRRepository,
		AWSRegion:     cfg.AWS.Region,
		AWSAccountID:  cfg.AWS.AccountID,
		ImageTag:      cfg.Container.Tag,
	})
}
