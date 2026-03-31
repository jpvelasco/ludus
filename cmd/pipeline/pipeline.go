package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/spf13/cobra"
)

var (
	skipEngine    bool
	skipGame      bool
	skipContainer bool
	skipDeploy    bool
	withClient    bool
	withSession   bool
	backend       string
	noCache       bool
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
Use --backend docker to build engine and game inside Docker.
Use the global --dry-run flag to see what commands would be executed.`,
	RunE: runPipeline,
}

func init() {
	Cmd.Flags().BoolVar(&skipEngine, "skip-engine", false, "skip engine build (use existing build)")
	Cmd.Flags().BoolVar(&skipGame, "skip-game", false, "skip game build (use existing build)")
	Cmd.Flags().BoolVar(&skipContainer, "skip-container", false, "skip container build and push (use existing image)")
	Cmd.Flags().BoolVar(&skipDeploy, "skip-deploy", false, "skip deployment (build only)")
	Cmd.Flags().BoolVar(&withClient, "with-client", false, "also build a standalone Linux game client")
	Cmd.Flags().BoolVar(&withSession, "with-session", false, "create a game session after deployment")
	Cmd.Flags().StringVar(&backend, "backend", "", `build backend: "native" or "docker" (default: from ludus.yaml)`)
	Cmd.Flags().BoolVar(&noCache, "no-cache", false, "disable build caching (force rebuild of all stages)")
}

type pipelineStage struct {
	name string
	skip bool
	fn   func(ctx context.Context) error
}

func runPipeline(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg
	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)

	// Resolve the deployment target to determine which stages are needed
	target, err := globals.ResolveTarget(cmd.Context(), cfg, "")
	if err != nil {
		return fmt.Errorf("resolving deploy target: %w", err)
	}
	caps := target.Capabilities()

	// Derive server build directory from project path
	arch := cfg.Game.ResolvedArch()
	projectPath := cfg.Game.ProjectPath
	if projectPath == "" && cfg.Game.ProjectName == "Lyra" {
		projectPath = filepath.Join(cfg.Engine.SourcePath,
			"Samples", "Games", "Lyra", "Lyra.uproject")
	}
	serverBuildDir := filepath.Join(filepath.Dir(projectPath),
		"PackagedServer", config.ServerPlatformDir(arch))

	// Compute cache hashes upfront
	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)
	clientHash := cache.GameClientKey(cfg, engineHash, "Linux")

	// Load cache once for pipeline-wide checks
	buildCache, _ := cache.Load()

	p := &pipelineCtx{
		cfg:            cfg,
		r:              r,
		engineVersion:  engineVersion,
		useDocker:      resolveBackend() == "docker",
		arch:           arch,
		serverBuildDir: serverBuildDir,
		target:         target,
		engineHash:     engineHash,
		serverHash:     serverHash,
		clientHash:     clientHash,
		buildCache:     buildCache,
	}

	projectName := cfg.Game.ProjectName
	stages := []pipelineStage{
		{name: "Validate prerequisites", fn: p.stageValidate},
		{name: "Build Unreal Engine", skip: skipEngine, fn: p.stageEngineBuild},
		{name: fmt.Sprintf("Build %s server (%s)", projectName, config.UEPlatformName(arch)), skip: skipGame, fn: p.stageGameBuild},
		{name: fmt.Sprintf("Build %s client (Linux)", projectName), skip: !withClient, fn: p.stageClientBuild},
		{name: "Build container image", skip: skipContainer || !caps.NeedsContainerBuild, fn: p.stageContainerBuild},
		{name: "Push to Amazon ECR", skip: skipContainer || !caps.NeedsContainerPush, fn: p.stageContainerPush},
		{name: fmt.Sprintf("Deploy to %s", target.Name()), skip: skipDeploy, fn: p.stageDeploy},
		{name: "Create game session", skip: skipDeploy || !withSession || !caps.SupportsSession, fn: p.stageSession},
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
	if withSession {
		fmt.Println("\nNext: ludus connect")
	} else if !skipDeploy {
		fmt.Println("\nNext: ludus deploy session")
	}
	return nil
}
