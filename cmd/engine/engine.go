package engine

import (
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	engBuilder "github.com/devrecon/ludus/internal/engine"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/spf13/cobra"
)

var (
	uePath string
	jobs   int
)

// Cmd is the top-level engine command group.
var Cmd = &cobra.Command{
	Use:   "engine",
	Short: "Build and manage Unreal Engine from source",
	Long: `Commands for building Unreal Engine from source. This handles running
Setup.sh to download dependencies, generating project files, and compiling
the engine for the target platform.`,
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Unreal Engine from source",
	Long: `Runs the full engine build pipeline:

  1. Run Setup.sh to download dependencies
  2. Generate project files
  3. Compile the engine (Development Editor + Server targets)

Use --jobs to control build parallelism (lower values use less memory).`,
	RunE: runBuild,
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run Setup.sh to download engine dependencies",
	RunE:  runSetup,
}

func init() {
	Cmd.PersistentFlags().StringVar(&uePath, "path", "", "path to Unreal Engine source (default: from ludus.yaml)")

	buildCmd.Flags().IntVarP(&jobs, "jobs", "j", 0, "max parallel compile jobs (0 = auto-detect based on available RAM)")

	Cmd.AddCommand(buildCmd)
	Cmd.AddCommand(setupCmd)
}

func makeBuilder() (*engBuilder.Builder, error) {
	cfg := globals.Cfg
	sourcePath := uePath
	if sourcePath == "" {
		sourcePath = cfg.Engine.SourcePath
	}
	if sourcePath == "" {
		return nil, fmt.Errorf("engine source path not configured (set engine.sourcePath in ludus.yaml or use --path)")
	}

	maxJobs := jobs
	if maxJobs == 0 {
		maxJobs = cfg.Engine.MaxJobs
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	return engBuilder.NewBuilder(engBuilder.BuildOptions{
		SourcePath: sourcePath,
		MaxJobs:    maxJobs,
		Verbose:    globals.Verbose,
	}, r), nil
}

func runSetup(cmd *cobra.Command, args []string) error {
	builder, err := makeBuilder()
	if err != nil {
		return err
	}

	fmt.Println("Running engine setup (Setup.sh)...")
	return builder.Setup(cmd.Context())
}

func runBuild(cmd *cobra.Command, args []string) error {
	builder, err := makeBuilder()
	if err != nil {
		return err
	}

	fmt.Println("Building Unreal Engine from source...")
	result, err := builder.Build(cmd.Context())
	if err != nil {
		return err
	}

	fmt.Printf("Engine build complete in %.0fs at %s\n", result.Duration, result.EnginePath)
	return nil
}
