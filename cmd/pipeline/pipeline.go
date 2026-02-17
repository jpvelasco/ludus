package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	ctrBuilder "github.com/devrecon/ludus/internal/container"
	engBuilder "github.com/devrecon/ludus/internal/engine"
	"github.com/devrecon/ludus/internal/gamelift"
	lyraBuilder "github.com/devrecon/ludus/internal/lyra"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/spf13/cobra"
)

var (
	skipEngine    bool
	skipLyra      bool
	skipContainer bool
	skipDeploy    bool
)

// Cmd is the full pipeline command.
var Cmd = &cobra.Command{
	Use:   "run",
	Short: "Run the full pipeline end-to-end",
	Long: `Executes the complete Ludus pipeline:

  1. Validate prerequisites (ludus init)
  2. Build Unreal Engine from source (ludus engine build)
  3. Build Lyra dedicated server for Linux (ludus lyra build)
  4. Build Docker container image (ludus container build)
  5. Push to Amazon ECR (ludus container push)
  6. Deploy to GameLift Containers (ludus deploy fleet)

Use --skip-* flags to skip stages that are already complete.
Use the global --dry-run flag to see what commands would be executed.`,
	RunE: runPipeline,
}

func init() {
	Cmd.Flags().BoolVar(&skipEngine, "skip-engine", false, "skip engine build (use existing build)")
	Cmd.Flags().BoolVar(&skipLyra, "skip-lyra", false, "skip Lyra build (use existing build)")
	Cmd.Flags().BoolVar(&skipContainer, "skip-container", false, "skip container build and push (use existing image)")
	Cmd.Flags().BoolVar(&skipDeploy, "skip-deploy", false, "skip deployment (build only)")
}

type stage struct {
	name string
	skip bool
	fn   func(ctx context.Context) error
}

func runPipeline(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg
	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	stages := []stage{
		{
			name: "Validate prerequisites",
			fn: func(ctx context.Context) error {
				checker := prereq.NewChecker(cfg.Engine.SourcePath)
				results := checker.RunAll()
				failed := 0
				for _, res := range results {
					marker := "[OK]"
					if !res.Passed {
						marker = "[FAIL]"
						failed++
					}
					fmt.Printf("    %-6s %s\n", marker, res.Name)
				}
				if failed > 0 {
					return fmt.Errorf("%d prerequisite check(s) failed", failed)
				}
				return nil
			},
		},
		{
			name: "Build Unreal Engine",
			skip: skipEngine,
			fn: func(ctx context.Context) error {
				builder := engBuilder.NewBuilder(engBuilder.BuildOptions{
					SourcePath: cfg.Engine.SourcePath,
					MaxJobs:    cfg.Engine.MaxJobs,
					Verbose:    globals.Verbose,
				}, r)
				result, err := builder.Build(ctx)
				if err != nil {
					return err
				}
				fmt.Printf("    Engine built in %.0fs\n", result.Duration)
				return nil
			},
		},
		{
			name: "Build Lyra server (Linux)",
			skip: skipLyra,
			fn: func(ctx context.Context) error {
				builder := lyraBuilder.NewBuilder(lyraBuilder.BuildOptions{
					EnginePath:  cfg.Engine.SourcePath,
					ProjectPath: cfg.Lyra.ProjectPath,
					Platform:    cfg.Lyra.Platform,
					ServerOnly:  true,
					ServerMap:   cfg.Lyra.ServerMap,
				}, r)
				result, err := builder.Build(ctx)
				if err != nil {
					return err
				}
				fmt.Printf("    Lyra server built in %.0fs at %s\n", result.Duration, result.OutputDir)
				return nil
			},
		},
		{
			name: "Build container image",
			skip: skipContainer,
			fn: func(ctx context.Context) error {
				serverDir := filepath.Join(cfg.Engine.SourcePath,
					"Samples", "Games", "Lyra", "PackagedServer", "LinuxServer")
				builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
					ServerBuildDir: serverDir,
					ImageName:      cfg.Container.ImageName,
					Tag:            cfg.Container.Tag,
					ServerPort:     cfg.Container.ServerPort,
				}, r)
				result, err := builder.Build(ctx)
				if err != nil {
					return err
				}
				fmt.Printf("    Image built: %s (%.0fs)\n", result.ImageTag, result.Duration)
				return nil
			},
		},
		{
			name: "Push to Amazon ECR",
			skip: skipContainer,
			fn: func(ctx context.Context) error {
				builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
					ImageName:  cfg.Container.ImageName,
					Tag:        cfg.Container.Tag,
					ServerPort: cfg.Container.ServerPort,
				}, r)
				return builder.Push(ctx, ctrBuilder.PushOptions{
					ECRRepository: cfg.AWS.ECRRepository,
					AWSRegion:     cfg.AWS.Region,
					AWSAccountID:  cfg.AWS.AccountID,
					ImageTag:      cfg.Container.Tag,
				})
			},
		},
		{
			name: "Deploy to GameLift",
			skip: skipDeploy,
			fn: func(ctx context.Context) error {
				awsCfg, err := gamelift.LoadAWSConfig(ctx, cfg.AWS.Region)
				if err != nil {
					return fmt.Errorf("loading AWS config: %w", err)
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
				}, awsCfg)

				fmt.Println("    Creating container group definition...")
				cgdARN, err := deployer.CreateContainerGroupDefinition(ctx)
				if err != nil {
					return err
				}

				fmt.Println("    Creating fleet...")
				status, err := deployer.CreateFleet(ctx, cgdARN)
				if err != nil {
					return err
				}
				fmt.Printf("    Fleet %s is %s\n", status.FleetID, status.Status)
				return nil
			},
		},
	}

	// Dry-run mode: print the plan, then execute with runner in dry-run mode
	if globals.DryRun {
		fmt.Println("Dry run — would execute:")
		fmt.Println()
	}

	// Execute stages
	total := len(stages)
	for i, s := range stages {
		if s.skip {
			fmt.Printf("[%d/%d] %s (skipped)\n", i+1, total, s.name)
			continue
		}

		fmt.Printf("[%d/%d] %s...\n", i+1, total, s.name)
		start := time.Now()

		if err := s.fn(cmd.Context()); err != nil {
			fmt.Printf("\nPipeline failed at stage %d/%d: %s\n", i+1, total, s.name)
			return fmt.Errorf("stage %q failed: %w", s.name, err)
		}

		elapsed := time.Since(start)
		fmt.Printf("[%d/%d] %s complete (%s)\n\n", i+1, total, s.name, elapsed.Truncate(time.Second))
	}

	fmt.Println("Pipeline complete.")
	return nil
}
