package container

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	ctrBuilder "github.com/devrecon/ludus/internal/container"
	"github.com/devrecon/ludus/internal/diagnose"
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
	Short: "Containerize the game dedicated server",
	Long: `Commands for building and managing the Docker container image
for the game dedicated server. Uses the GameLift Containers
Starter Kit patterns (Amazon Linux 2023 base, non-root user).`,
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the Docker container image",
	Long: `Builds a Docker image containing:

  - Amazon Linux 2023 base image
  - Game dedicated server binary (Linux)
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

// resolveServerBuildDir determines the server build directory from config.
func resolveServerBuildDir() string {
	cfg := globals.Cfg
	platformDir := config.ServerPlatformDir(cfg.Game.ResolvedArch())
	if cfg.Game.ProjectPath != "" {
		return filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "PackagedServer", platformDir)
	}
	if cfg.Engine.SourcePath != "" && cfg.Game.ProjectName == "Lyra" {
		return filepath.Join(cfg.Engine.SourcePath, "Samples", "Games", "Lyra", "PackagedServer", platformDir)
	}
	return ""
}

func runBuild(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	serverBuildDir := resolveServerBuildDir()
	containerHash := cache.ContainerKey(cfg, serverBuildDir)

	if !noCache {
		c, err := cache.Load()
		if err == nil && c.IsHit(cache.StageContainerBuild, containerHash) {
			fmt.Println("Container image is up to date (cached), skipping.")
			return nil
		}
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
		ServerBuildDir: serverBuildDir,
		ImageName:      cfg.Container.ImageName,
		Tag:            tag,
		ServerPort:     cfg.Container.ServerPort,
		NoCache:        noCache,
	}, r)

	fmt.Println("Building container image...")
	result, err := builder.Build(cmd.Context())
	if err != nil {
		return diagnose.ContainerError(err, "container build")
	}

	if c, cErr := cache.Load(); cErr == nil {
		c.Set(cache.StageContainerBuild, containerHash, time.Now().UTC().Format(time.RFC3339))
		_ = cache.Save(c)
	}

	fmt.Printf("Container image built: %s (%.0fs)\n", result.ImageTag, result.Duration)
	fmt.Println("\nNext: ludus container push")
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
	if err := builder.Push(cmd.Context(), ctrBuilder.PushOptions{
		ECRRepository: cfg.AWS.ECRRepository,
		AWSRegion:     cfg.AWS.Region,
		AWSAccountID:  cfg.AWS.AccountID,
		ImageTag:      cfg.Container.Tag,
	}); err != nil {
		return diagnose.ContainerError(err, "container push")
	}
	fmt.Println("\nNext: ludus deploy fleet  (or: ludus deploy stack)")
	return nil
}
