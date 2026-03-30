package container

import (
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	ctrBuilder "github.com/devrecon/ludus/internal/container"
	"github.com/devrecon/ludus/internal/dflint"
	"github.com/devrecon/ludus/internal/diagnose"
	"github.com/devrecon/ludus/internal/ecr"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/spf13/cobra"
)

var (
	tag      string
	pushTag  string
	noCache  bool
	archFlag string
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
	buildCmd.Flags().StringVar(&archFlag, "arch", "", `target CPU architecture: amd64, arm64 (default: from ludus.yaml)`)

	pushCmd.Flags().StringVarP(&pushTag, "tag", "t", "", "image tag to push (default: from ludus.yaml or latest)")

	Cmd.AddCommand(buildCmd)
	Cmd.AddCommand(pushCmd)
}

// resolveArch returns the effective architecture, preferring CLI flag over config.
func resolveArch() string {
	if archFlag != "" {
		return config.NormalizeArch(archFlag)
	}
	return globals.Cfg.Game.ResolvedArch()
}

func runBuild(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	// Apply arch flag to config so ResolveServerBuildDir sees it
	if archFlag != "" {
		cfg.Game.Arch = archFlag
	}

	checker := prereq.NewChecker(cfg.Engine.SourcePath, cfg.Engine.Version, false, &cfg.Game)
	if err := prereq.Validate(checker.CheckDockerReady()); err != nil {
		return err
	}

	serverBuildDir := config.ResolveServerBuildDir(cfg)
	containerHash := cache.ContainerKey(cfg, serverBuildDir)

	if !noCache {
		c, err := cache.Load()
		if err == nil {
			if c.IsHit(cache.StageContainerBuild, containerHash) {
				fmt.Println("Container image is up to date (cached), skipping.")
				return nil
			}
			if reason := c.MissReason(cache.StageContainerBuild, containerHash); reason != "" {
				fmt.Printf("Cache: %s\n", reason)
			}
		}
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
		ServerBuildDir: serverBuildDir,
		ImageName:      cfg.Container.ImageName,
		Tag:            tag,
		ServerPort:     cfg.Container.ServerPort,
		NoCache:        noCache,
		ProjectName:    cfg.Game.ProjectName,
		ServerTarget:   cfg.Game.ResolvedServerTarget(),
		Arch:           resolveArch(),
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

	// Quick security lint of generated Dockerfile (built-in rules only)
	lintResult := dflint.LintDockerfile(builder.GenerateDockerfile())
	if lintResult.HasWarnings() {
		fmt.Printf("  Security: %s\n", lintResult.Summary())
	}

	fmt.Printf("Container image built: %s (%.0fs)\n", result.ImageTag, result.Duration)
	fmt.Println("\nNext: ludus container push")
	return nil
}

func runPush(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	checker := prereq.NewChecker(cfg.Engine.SourcePath, cfg.Engine.Version, false, &cfg.Game)
	if err := prereq.Validate(checker.CheckPushReady()); err != nil {
		return err
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	imageTag := pushTag
	if imageTag == "" {
		imageTag = cfg.Container.Tag
	}

	builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
		ImageName:  cfg.Container.ImageName,
		Tag:        imageTag,
		ServerPort: cfg.Container.ServerPort,
	}, r)

	fmt.Println("Pushing container image to ECR...")
	if err := builder.Push(cmd.Context(), ecr.PushOptions{
		ECRRepository: cfg.AWS.ECRRepository,
		AWSRegion:     cfg.AWS.Region,
		AWSAccountID:  cfg.AWS.AccountID,
		ImageTag:      imageTag,
	}); err != nil {
		return diagnose.ContainerError(err, "container push")
	}
	fmt.Println("\nNext: ludus deploy fleet  (or: ludus deploy stack)")
	return nil
}
