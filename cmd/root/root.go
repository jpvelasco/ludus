package root

import (
	"fmt"
	"os"

	"github.com/devrecon/ludus/cmd/buildgraph"
	"github.com/devrecon/ludus/cmd/ci"
	"github.com/devrecon/ludus/cmd/configcmd"
	"github.com/devrecon/ludus/cmd/connect"
	"github.com/devrecon/ludus/cmd/container"
	"github.com/devrecon/ludus/cmd/deploy"
	"github.com/devrecon/ludus/cmd/doctor"
	"github.com/devrecon/ludus/cmd/engine"
	"github.com/devrecon/ludus/cmd/game"
	"github.com/devrecon/ludus/cmd/globals"
	ludusmcp "github.com/devrecon/ludus/cmd/mcp"
	"github.com/devrecon/ludus/cmd/pipeline"
	"github.com/devrecon/ludus/cmd/setup"
	"github.com/devrecon/ludus/cmd/status"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/devrecon/ludus/internal/version"
	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:          "ludus",
	Version:      version.Version,
	SilenceUsage: true,
	Short:        "Streamline UE5 dedicated server deployment to AWS GameLift Containers",
	Long: `Ludus automates the end-to-end pipeline for building Unreal Engine 5 from source,
compiling a UE5 game project as a Linux dedicated server, containerizing it,
and deploying it to AWS GameLift Containers.

  ludus setup       Interactive setup wizard (first-time configuration)
  ludus init        Validate prerequisites and configure the environment
  ludus doctor      Run comprehensive diagnostics
  ludus config      View and modify ludus.yaml configuration
  ludus engine      Build Unreal Engine from source
  ludus game        Build the game as a Linux dedicated server
  ludus container   Containerize the server build
  ludus deploy      Deploy the container to AWS GameLift
  ludus connect     Launch the game client and connect to a game session
  ludus status      Check status of all pipeline stages
  ludus run         Run the full pipeline end-to-end
  ludus ci          Generate CI workflows and manage runners

Use --profile to manage multiple configurations (e.g., different UE versions):
  ludus --profile ue57-ec2 setup
  ludus --profile ue57-ec2 run`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Activate state profile before any state I/O
		state.SetProfile(globals.Profile)

		// Try profile-specific config first: ludus-<profile>.yaml
		cfgPath := cfgFile
		if cfgPath == "" && globals.Profile != "" {
			profileCfg := "ludus-" + globals.Profile + ".yaml"
			if _, err := os.Stat(profileCfg); err == nil {
				cfgPath = profileCfg
				if globals.Verbose {
					fmt.Fprintf(os.Stderr, "Using profile config: %s\n", profileCfg)
				}
			}
		}

		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		globals.Cfg = cfg

		// Auto-detect engine version from Build.version if not set in config
		if cfg.Engine.SourcePath != "" && cfg.Engine.Version == "" {
			if bv, err := toolchain.ParseBuildVersion(cfg.Engine.SourcePath); err == nil {
				cfg.Engine.Version = fmt.Sprintf("%d.%d.%d", bv.MajorVersion, bv.MinorVersion, bv.PatchVersion)
				if globals.Verbose {
					fmt.Fprintf(os.Stderr, "Auto-detected engine version: %s\n", cfg.Engine.Version)
				}
			}
		}

		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./ludus.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&globals.Verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&globals.JSONOutput, "json", false, "output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&globals.DryRun, "dry-run", false, "print commands without executing")
	rootCmd.PersistentFlags().StringVar(&globals.Profile, "profile", "", "state profile for multi-version workflows (e.g., ue57-ec2)")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(setup.Cmd)
	rootCmd.AddCommand(configcmd.Cmd)
	rootCmd.AddCommand(doctor.Cmd)
	rootCmd.AddCommand(engine.Cmd)
	rootCmd.AddCommand(game.Cmd)
	rootCmd.AddCommand(container.Cmd)
	rootCmd.AddCommand(deploy.Cmd)
	rootCmd.AddCommand(connect.Cmd)
	rootCmd.AddCommand(status.Cmd)
	rootCmd.AddCommand(pipeline.Cmd)
	rootCmd.AddCommand(ludusmcp.Cmd)
	rootCmd.AddCommand(ci.Cmd)
	rootCmd.AddCommand(buildgraph.Cmd)
}
