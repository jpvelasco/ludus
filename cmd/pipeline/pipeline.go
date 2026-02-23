package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	ctrBuilder "github.com/devrecon/ludus/internal/container"
	"github.com/devrecon/ludus/internal/deploy"
	engBuilder "github.com/devrecon/ludus/internal/engine"
	gameBuilder "github.com/devrecon/ludus/internal/game"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/spf13/cobra"
)

var (
	skipEngine    bool
	skipGame      bool
	skipContainer bool
	skipDeploy    bool
	withClient    bool
)

// Cmd is the full pipeline command.
var Cmd = &cobra.Command{
	Use:   "run",
	Short: "Run the full pipeline end-to-end",
	Long: `Executes the complete Ludus pipeline:

  1. Validate prerequisites (ludus init)
  2. Build Unreal Engine from source (ludus engine build)
  3. Build game dedicated server for Linux (ludus game build)
  4. Build Docker container image (ludus container build)  [if target requires it]
  5. Push to Amazon ECR (ludus container push)              [if target requires it]
  6. Deploy to target (ludus deploy)

Use --skip-* flags to skip stages that are already complete.
Use the global --dry-run flag to see what commands would be executed.`,
	RunE: runPipeline,
}

func init() {
	Cmd.Flags().BoolVar(&skipEngine, "skip-engine", false, "skip engine build (use existing build)")
	Cmd.Flags().BoolVar(&skipGame, "skip-game", false, "skip game build (use existing build)")
	Cmd.Flags().BoolVar(&skipContainer, "skip-container", false, "skip container build and push (use existing image)")
	Cmd.Flags().BoolVar(&skipDeploy, "skip-deploy", false, "skip deployment (build only)")
	Cmd.Flags().BoolVar(&withClient, "with-client", false, "also build a standalone Linux game client")
}

type pipelineStage struct {
	name string
	skip bool
	fn   func(ctx context.Context) error
}

func runPipeline(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg
	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	projectName := cfg.Game.ProjectName

	// Resolve the deployment target to determine which stages are needed
	target, err := globals.ResolveTarget(cmd.Context(), cfg, "")
	if err != nil {
		return fmt.Errorf("resolving deploy target: %w", err)
	}
	caps := target.Capabilities()

	// Derive server build directory from project path
	projectPath := cfg.Game.ProjectPath
	if projectPath == "" && cfg.Game.ProjectName == "Lyra" {
		projectPath = filepath.Join(cfg.Engine.SourcePath,
			"Samples", "Games", "Lyra", "Lyra.uproject")
	}
	serverBuildDir := filepath.Join(filepath.Dir(projectPath),
		"PackagedServer", "LinuxServer")

	stages := []pipelineStage{
		{
			name: "Validate prerequisites",
			fn: func(ctx context.Context) error {
				checker := prereq.NewChecker(cfg.Engine.SourcePath, cfg.Engine.Version, false, &cfg.Game)
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
			name: fmt.Sprintf("Build %s server (Linux)", projectName),
			skip: skipGame,
			fn: func(ctx context.Context) error {
				builder := gameBuilder.NewBuilder(gameBuilder.BuildOptions{
					EnginePath:   cfg.Engine.SourcePath,
					ProjectPath:  cfg.Game.ProjectPath,
					ProjectName:  cfg.Game.ProjectName,
					ServerTarget: cfg.Game.ResolvedServerTarget(),
					GameTarget:   cfg.Game.ResolvedGameTarget(),
					Platform:     cfg.Game.Platform,
					ServerOnly:   true,
					ServerMap:    cfg.Game.ServerMap,
				}, r)
				result, err := builder.Build(ctx)
				if err != nil {
					return err
				}
				fmt.Printf("    %s server built in %.0fs at %s\n", projectName, result.Duration, result.OutputDir)
				return nil
			},
		},
		{
			name: fmt.Sprintf("Build %s client (Linux)", projectName),
			skip: !withClient,
			fn: func(ctx context.Context) error {
				builder := gameBuilder.NewBuilder(gameBuilder.BuildOptions{
					EnginePath:   cfg.Engine.SourcePath,
					ProjectPath:  cfg.Game.ProjectPath,
					ProjectName:  cfg.Game.ProjectName,
					ClientTarget: cfg.Game.ResolvedClientTarget(),
					Platform:     cfg.Game.Platform,
				}, r)
				result, err := builder.BuildClient(ctx)
				if err != nil {
					return err
				}
				if err := state.UpdateClient(&state.ClientState{
					BinaryPath: result.ClientBinary,
					OutputDir:  result.OutputDir,
					BuiltAt:    time.Now().UTC().Format(time.RFC3339),
				}); err != nil {
					fmt.Printf("    Warning: failed to write state: %v\n", err)
				}
				fmt.Printf("    %s client built in %.0fs at %s\n", projectName, result.Duration, result.OutputDir)
				return nil
			},
		},
		{
			name: "Build container image",
			skip: skipContainer || !caps.NeedsContainerBuild,
			fn: func(ctx context.Context) error {
				builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
					ServerBuildDir: serverBuildDir,
					ImageName:      cfg.Container.ImageName,
					Tag:            cfg.Container.Tag,
					ServerPort:     cfg.Container.ServerPort,
					ProjectName:    cfg.Game.ProjectName,
					ServerTarget:   cfg.Game.ResolvedServerTarget(),
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
			skip: skipContainer || !caps.NeedsContainerPush,
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
			name: fmt.Sprintf("Deploy to %s", target.Name()),
			skip: skipDeploy,
			fn: func(ctx context.Context) error {
				imageURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
					cfg.AWS.AccountID, cfg.AWS.Region, cfg.AWS.ECRRepository, cfg.Container.Tag)

				result, err := target.Deploy(ctx, deploy.DeployInput{
					ImageURI:       imageURI,
					ServerBuildDir: serverBuildDir,
					ServerPort:     cfg.Container.ServerPort,
				})
				if err != nil {
					return err
				}

				if err := state.UpdateDeploy(&state.DeployState{
					TargetName: result.TargetName,
					Status:     result.Status,
					Detail:     result.Detail,
					DeployedAt: time.Now().UTC().Format(time.RFC3339),
				}); err != nil {
					fmt.Printf("    Warning: failed to write state: %v\n", err)
				}

				fmt.Printf("    Deployed to %s: %s\n", result.TargetName, result.Detail)
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
