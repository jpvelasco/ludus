package deploy

import (
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/diagnose"
	"github.com/devrecon/ludus/internal/gamelift"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/devrecon/ludus/internal/pricing"
	"github.com/devrecon/ludus/internal/state"
	"github.com/spf13/cobra"
)

var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Create or update a GameLift container fleet",
	Long: `Deploys the container to GameLift by:

  1. Creating a container group definition
  2. Waiting for the image to be snapshotted (COPYING -> READY)
  3. Creating/updating the container fleet
  4. Configuring inbound permissions (UDP 7777)`,
	RunE: runFleet,
}

func init() {
	fleetCmd.Flags().BoolVar(&withSession, "with-session", false, "create a game session after deployment")
	Cmd.AddCommand(fleetCmd)
}

func runFleet(cmd *cobra.Command, args []string) error {
	checker := prereq.NewChecker(globals.Cfg.Engine.SourcePath, globals.Cfg.Engine.Version, false, &globals.Cfg.Game)
	if err := prereq.Validate(checker.CheckAWSReady()); err != nil {
		return err
	}

	deployer, err := makeDeployer(cmd)
	if err != nil {
		return err
	}

	it := instanceType
	if it == "" {
		it = globals.Cfg.GameLift.InstanceType
	}
	if est := pricing.FormatEstimate(it); est != "" {
		fmt.Println(est)
	}
	if sug := pricing.FormatSuggestion(it, globals.Cfg.Game.ResolvedArch()); sug != "" {
		fmt.Println(sug)
	}

	fmt.Println("Creating container group definition...")
	cgdARN, err := deployer.CreateContainerGroupDefinition(cmd.Context())
	if err != nil {
		return diagnose.DeployError(err, "gamelift")
	}
	fmt.Printf("Container group definition ready: %s\n\n", cgdARN)

	fmt.Println("Creating container fleet...")
	fleetStatus, err := deployer.CreateFleet(cmd.Context(), cgdARN)
	if err != nil {
		return diagnose.DeployError(err, "gamelift")
	}

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   fleetStatus.FleetID,
		Status:    fleetStatus.Status,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}

	fmt.Printf("\nFleet deployed: %s (status: %s)\n", fleetStatus.FleetID, fleetStatus.Status)
	if err := maybeCreateSession(cmd.Context(), gamelift.NewTargetAdapter(deployer)); err != nil {
		return err
	}
	if !withSession {
		fmt.Println("\nNext: ludus deploy session")
	} else {
		fmt.Println("\nNext: ludus connect")
	}
	return nil
}
