package deploy

import (
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/diagnose"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/spf13/cobra"
)

var anywhereCmd = &cobra.Command{
	Use:   "anywhere",
	Short: "Deploy a local Anywhere fleet and launch the game server",
	Long: `Creates a GameLift Anywhere fleet, registers this machine as a compute,
and launches the game server via the GameLift Game Server Wrapper.

The server runs locally but GameLift manages sessions, matchmaking, and
player validation. Fleet creation takes seconds, not minutes.

Use --ip to override the auto-detected local IP address.`,
	RunE: runAnywhere,
}

func init() {
	anywhereCmd.Flags().StringVar(&anywhereIP, "ip", "", "local IP address override (default: auto-detect)")
	anywhereCmd.Flags().BoolVar(&withSession, "with-session", false, "create a game session after deployment")
	Cmd.AddCommand(anywhereCmd)
}

func applyAnywhereFlags(cfg *config.Config) {
	if region != "" {
		cfg.AWS.Region = region
	}
	if fleetName != "" {
		cfg.GameLift.FleetName = fleetName
	}
	if anywhereIP != "" {
		cfg.Anywhere.IPAddress = anywhereIP
	}
}

func runAnywhere(cmd *cobra.Command, args []string) error {
	cfg := *globals.Cfg
	applyAnywhereFlags(&cfg)

	checker := prereq.NewChecker(cfg.Engine.SourcePath, cfg.Engine.Version, false, &cfg.Game)
	if err := prereq.Validate(checker.CheckAWSReady()); err != nil {
		return err
	}

	target, err := globals.ResolveTarget(cmd.Context(), &cfg, "anywhere")
	if err != nil {
		return err
	}

	result, err := target.Deploy(cmd.Context(), deploy.DeployInput{
		ServerPort: cfg.Container.ServerPort,
	})
	if err != nil {
		return diagnose.DeployError(err, "anywhere")
	}

	fmt.Printf("\nAnywhere deployment ready: %s\n", result.Detail)
	if sm, ok := target.(deploy.SessionManager); ok {
		if err := maybeCreateSession(cmd.Context(), sm); err != nil {
			return err
		}
	}
	printNextStep()
	return nil
}
