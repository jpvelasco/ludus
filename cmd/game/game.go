package game

import (
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	gameBuilder "github.com/devrecon/ludus/internal/game"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/spf13/cobra"
)

var (
	skipCook       bool
	skipCookClient bool
	clientPlatform string
)

// Cmd is the top-level game command group.
var Cmd = &cobra.Command{
	Use:   "game",
	Short: "Build and configure the UE5 game dedicated server",
	Long: `Commands for building a UE5 project as a dedicated server.
This handles compiling the server target, cooking content for Linux,
and integrating the GameLift Server SDK.`,
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the game as a Linux dedicated server",
	Long: `Builds the UE5 project using RunUAT BuildCookRun:

  1. Build the server target for Linux
  2. Cook content for the Linux server platform
  3. Stage and package the server build
  4. Output a ready-to-containerize server directory`,
	RunE: runBuild,
}

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Build the game as a standalone game client",
	Long: `Builds the UE5 project using RunUAT BuildCookRun as a game client:

  1. Build the game client target for the specified platform
  2. Cook content for the client platform
  3. Stage and package the client build
  4. Output a ready-to-run client directory

Use --platform to target a different platform (default: Linux).
Win64 cross-compilation requires the Windows cross-compile toolchain.`,
	RunE: runClientBuild,
}

var integrateCmd = &cobra.Command{
	Use:   "integrate-gamelift",
	Short: "Integrate GameLift Server SDK into the project",
	Long: `Patches the UE5 project to include the GameLift Server SDK:

  - Adds GameLiftServerSDK module dependency to the game Build.cs
  - Creates a GameLift-aware GameMode subclass
  - Configures server startup to call InitSDK and ProcessReady`,
	RunE: runIntegrate,
}

func init() {
	buildCmd.Flags().BoolVar(&skipCook, "skip-cook", false, "skip content cooking (use previously cooked content)")
	clientCmd.Flags().BoolVar(&skipCookClient, "skip-cook", false, "skip content cooking (use previously cooked content)")
	clientCmd.Flags().StringVar(&clientPlatform, "platform", "Linux", "target platform (Linux, Win64)")

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

	engineVersion, _ := toolchain.DetectEngineVersion(enginePath, cfg.Engine.Version)

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := gameBuilder.NewBuilder(gameBuilder.BuildOptions{
		EnginePath:    enginePath,
		ProjectPath:   cfg.Game.ProjectPath,
		ProjectName:   cfg.Game.ProjectName,
		ServerTarget:  cfg.Game.ResolvedServerTarget(),
		GameTarget:    cfg.Game.ResolvedGameTarget(),
		Platform:      cfg.Game.Platform,
		ServerOnly:    true,
		SkipCook:      skipCook,
		ServerMap:     cfg.Game.ServerMap,
		EngineVersion: engineVersion,
	}, r)

	fmt.Printf("Building %s dedicated server...\n", cfg.Game.ProjectName)
	result, err := builder.Build(cmd.Context())
	if err != nil {
		return err
	}

	fmt.Printf("%s server build complete in %.0fs\n", cfg.Game.ProjectName, result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	return nil
}

func runClientBuild(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	enginePath := cfg.Engine.SourcePath
	if enginePath == "" {
		return fmt.Errorf("engine source path not configured (set engine.sourcePath in ludus.yaml)")
	}

	engineVersion, _ := toolchain.DetectEngineVersion(enginePath, cfg.Engine.Version)

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := gameBuilder.NewBuilder(gameBuilder.BuildOptions{
		EnginePath:     enginePath,
		ProjectPath:    cfg.Game.ProjectPath,
		ProjectName:    cfg.Game.ProjectName,
		ClientTarget:   cfg.Game.ResolvedClientTarget(),
		ClientPlatform: clientPlatform,
		SkipCook:       skipCookClient,
		EngineVersion:  engineVersion,
	}, r)

	fmt.Printf("Building %s standalone client for %s...\n", cfg.Game.ProjectName, clientPlatform)
	result, err := builder.BuildClient(cmd.Context())
	if err != nil {
		return err
	}

	if err := state.UpdateClient(&state.ClientState{
		BinaryPath: result.ClientBinary,
		OutputDir:  result.OutputDir,
		Platform:   result.Platform,
		BuiltAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}

	fmt.Printf("%s client build complete in %.0fs\n", cfg.Game.ProjectName, result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	fmt.Printf("Binary: %s\n", result.ClientBinary)
	return nil
}

func runIntegrate(cmd *cobra.Command, args []string) error {
	fmt.Println("GameLift SDK integration not yet implemented.")
	fmt.Println("The default approach uses a Go SDK wrapper (no game code changes needed).")
	return nil
}
