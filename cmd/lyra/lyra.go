package lyra

import (
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	lyraBuilder "github.com/devrecon/ludus/internal/lyra"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/spf13/cobra"
)

var (
	skipCook       bool
	skipCookClient bool
)

// Cmd is the top-level lyra command group.
var Cmd = &cobra.Command{
	Use:   "lyra",
	Short: "Build and configure the Lyra dedicated server",
	Long: `Commands for building the Lyra sample project as a dedicated server.
This handles compiling the server target, cooking content for Linux,
and integrating the GameLift Server SDK.`,
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Lyra as a Linux dedicated server",
	Long: `Builds the Lyra project using RunUAT BuildCookRun:

  1. Build the LyraServer target for Linux
  2. Cook content for the Linux server platform
  3. Stage and package the server build
  4. Output a ready-to-containerize server directory`,
	RunE: runBuild,
}

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Build Lyra as a standalone Linux game client",
	Long: `Builds the Lyra project using RunUAT BuildCookRun as a game client:

  1. Build the LyraGame target for Linux
  2. Cook content for the Linux client platform
  3. Stage and package the client build
  4. Output a ready-to-run client directory`,
	RunE: runClientBuild,
}

var integrateCmd = &cobra.Command{
	Use:   "integrate-gamelift",
	Short: "Integrate GameLift Server SDK into the Lyra project",
	Long: `Patches the Lyra project to include the GameLift Server SDK:

  - Adds GameLiftServerSDK module dependency to LyraGame.Build.cs
  - Creates a GameLift-aware GameMode subclass
  - Configures server startup to call InitSDK and ProcessReady`,
	RunE: runIntegrate,
}

func init() {
	buildCmd.Flags().BoolVar(&skipCook, "skip-cook", false, "skip content cooking (use previously cooked content)")
	clientCmd.Flags().BoolVar(&skipCookClient, "skip-cook", false, "skip content cooking (use previously cooked content)")

	Cmd.AddCommand(buildCmd)
	Cmd.AddCommand(clientCmd)
	Cmd.AddCommand(integrateCmd)
}

func runBuild(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	enginePath := cfg.Engine.SourcePath
	if enginePath == "" {
		return fmt.Errorf("engine source path not configured (set engine.sourcePath in ludus.yaml)")
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := lyraBuilder.NewBuilder(lyraBuilder.BuildOptions{
		EnginePath:  enginePath,
		ProjectPath: cfg.Lyra.ProjectPath,
		Platform:    cfg.Lyra.Platform,
		ServerOnly:  true,
		SkipCook:    skipCook,
		ServerMap:   cfg.Lyra.ServerMap,
	}, r)

	fmt.Println("Building Lyra dedicated server...")
	result, err := builder.Build(cmd.Context())
	if err != nil {
		return err
	}

	fmt.Printf("Lyra server build complete in %.0fs\n", result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	return nil
}

func runClientBuild(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	enginePath := cfg.Engine.SourcePath
	if enginePath == "" {
		return fmt.Errorf("engine source path not configured (set engine.sourcePath in ludus.yaml)")
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := lyraBuilder.NewBuilder(lyraBuilder.BuildOptions{
		EnginePath:  enginePath,
		ProjectPath: cfg.Lyra.ProjectPath,
		Platform:    cfg.Lyra.Platform,
		SkipCook:    skipCookClient,
	}, r)

	fmt.Println("Building Lyra standalone client...")
	result, err := builder.BuildClient(cmd.Context())
	if err != nil {
		return err
	}

	if err := state.UpdateClient(&state.ClientState{
		BinaryPath: result.ClientBinary,
		OutputDir:  result.OutputDir,
		BuiltAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}

	fmt.Printf("Lyra client build complete in %.0fs\n", result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	fmt.Printf("Binary: %s\n", result.ClientBinary)
	return nil
}

func runIntegrate(cmd *cobra.Command, args []string) error {
	fmt.Println("GameLift SDK integration not yet implemented.")
	fmt.Println("The default approach uses a Go SDK wrapper (no Lyra code changes needed).")
	return nil
}
