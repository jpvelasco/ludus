package deploy

import (
	"fmt"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
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
  4. Configuring inbound UDP permissions. Set gamelift.allowedCidr to a
     restrictive client CIDR for test fleets. When unset, GameLift manages
     the connection port range and opens it to 0.0.0.0/0.`,
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

	plan := deployer.Plan()
	printPricingHints(plan.InstanceType, globals.Cfg.Game.ResolvedArch())

	if globals.DryRun {
		printFleetPlan(plan)
		return nil
	}

	if w := gamelift.OpenCIDRWarning(globals.Cfg.GameLift.AllowedCIDR); w != "" {
		fmt.Println(w)
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

	recordFleetDeployState(fleetStatus)

	fmt.Printf("\nFleet deployed: %s (status: %s)\n", fleetStatus.FleetID, fleetStatus.Status)
	fmt.Printf("Inbound: UDP connection ports open to %s (%s)\n",
		config.ResolvedAllowedCIDR(globals.Cfg.GameLift.AllowedCIDR),
		fleetPortRangeDescription(globals.Cfg.GameLift.AllowedCIDR))
	if err := maybeCreateSession(cmd.Context(), gamelift.NewTargetAdapter(deployer)); err != nil {
		return err
	}
	printNextStep()
	return nil
}

// printFleetPlan renders the resolved deploy plan under --dry-run so the
// operator can review every value that would reach AWS before a billable
// create (#778). The tag line surfaces the duplicate-key problem from #775
// before any resource is created.
func printFleetPlan(plan gamelift.FleetPlan) {
	fmt.Println("Dry run — would create container group definition and fleet (no AWS calls made).")
	fmt.Printf("  Image:               %s\n", plan.ImageURI)
	fmt.Printf("  Container group:     %s\n", plan.ContainerGroupName)
	fmt.Printf("  Fleet:               %s\n", plan.FleetName)
	fmt.Printf("  IAM role:            %s\n", plan.IAMRoleName)
	fmt.Printf("  Instance type:       %s\n", plan.InstanceType)
	fmt.Printf("  Containers/instance: %d\n", plan.ContainersPerInstance)
	fmt.Printf("  Inbound CIDR:        %s (%s)\n",
		config.ResolvedAllowedCIDR(plan.AllowedCIDR),
		fleetPortRangeDescription(plan.AllowedCIDR))
	fmt.Printf("  Tags:                %v\n", plan.Tags)
}

func fleetPortRangeDescription(cidr string) string {
	if cidr == "" {
		return "GameLift-managed connection port range"
	}
	return "Ludus-configured connection port range starting at 4192"
}

func recordFleetDeployState(fleetStatus *gamelift.FleetStatus) {
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
}
