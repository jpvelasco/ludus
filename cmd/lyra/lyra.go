package lyra

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	platform   string
	serverOnly bool
	skipCook   bool
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
	buildCmd.Flags().StringVar(&platform, "platform", "linux", "target platform (linux)")
	buildCmd.Flags().BoolVar(&serverOnly, "server-only", true, "build only the server target (no client)")
	buildCmd.Flags().BoolVar(&skipCook, "skip-cook", false, "skip content cooking (use previously cooked content)")

	Cmd.AddCommand(buildCmd)
	Cmd.AddCommand(integrateCmd)
}

func runBuild(cmd *cobra.Command, args []string) error {
	fmt.Println("Lyra build not yet implemented.")
	fmt.Println()
	fmt.Println("This will:")
	fmt.Println("  1. Locate the Lyra project (from engine Samples/ or configured path)")
	fmt.Println("  2. Build LyraServer target for Linux via RunUAT BuildCookRun")
	fmt.Println("  3. Cook and package server content")
	fmt.Printf("  4. Platform: %s, Server-only: %t\n", platform, serverOnly)
	return nil
}

func runIntegrate(cmd *cobra.Command, args []string) error {
	fmt.Println("GameLift SDK integration not yet implemented.")
	fmt.Println()
	fmt.Println("This will:")
	fmt.Println("  1. Download/verify GameLift Plugin for Unreal")
	fmt.Println("  2. Add GameLiftServerSDK to LyraGame.Build.cs")
	fmt.Println("  3. Create GameLift-aware GameMode subclass")
	fmt.Println("  4. Configure SDK lifecycle (InitSDK, ProcessReady, etc.)")
	return nil
}
