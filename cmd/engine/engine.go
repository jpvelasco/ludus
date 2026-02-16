package engine

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	ueVersion string
	uePath    string
	jobs      int
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
	Cmd.PersistentFlags().StringVar(&ueVersion, "version", "", "UE version tag (e.g. 5.5, 5.7.3)")

	buildCmd.Flags().IntVarP(&jobs, "jobs", "j", 0, "max parallel compile jobs (0 = auto-detect based on available RAM)")

	Cmd.AddCommand(buildCmd)
	Cmd.AddCommand(setupCmd)
}

func runBuild(cmd *cobra.Command, args []string) error {
	fmt.Println("Engine build not yet implemented.")
	fmt.Println()
	fmt.Println("This will:")
	fmt.Println("  1. Validate engine source at the configured path")
	fmt.Println("  2. Run Setup.sh (download ~40GB of dependencies)")
	fmt.Println("  3. Generate project files")
	fmt.Println("  4. Compile the engine (Development Editor + Server)")
	fmt.Printf("  5. Parallel jobs: %d (0 = auto)\n", jobs)
	return nil
}

func runSetup(cmd *cobra.Command, args []string) error {
	fmt.Println("Engine setup not yet implemented.")
	fmt.Println("This will run Setup.sh to download engine dependencies.")
	return nil
}
