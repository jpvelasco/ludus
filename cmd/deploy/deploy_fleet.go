package deploy

import (
	"fmt"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/diagnose"
	"github.com/jpvelasco/ludus/internal/gamelift"
	"github.com/jpvelasco/ludus/internal/prereq"
	"github.com/jpvelasco/ludus/internal/state"
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
	printPricingHints(it, globals.Cfg.Game.ResolvedArch())

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

	now := time.Now().UTC().Format(time.RFC3339)

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   fleetStatus.FleetID,
		Status:    fleetStatus.Status,
		CreatedAt: now,
	}); err != nil {
		fmt.Printf("Warning: failed to write fleet state: %v\n", err)
	}

	detail := fmt.Sprintf("fleet %s", fleetStatus.FleetID)
	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "gamelift",
		Status:     fleetStatus.Status,
		Detail:     detail,
		DeployedAt: now,
	}); err != nil {
		fmt.Printf("Warning: failed to write deploy state: %v\n", err)
	}

	fmt.Printf("\nFleet deployed: %s (status: %s)\n", fleetStatus.FleetID, fleetStatus.Status)
	if err := maybeCreateSession(cmd.Context(), gamelift.NewTargetAdapter(deployer)); err != nil {
		return err
	}
	printNextStep()
	return nil
}
