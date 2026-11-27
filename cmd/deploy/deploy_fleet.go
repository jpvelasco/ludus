package deploy

import (
	"fmt"
	"os"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/diagnose"
	"github.com/jpvelasco/ludus/internal/gamelift"
	"github.com/jpvelasco/ludus/internal/prereq"
	"github.com/spf13/cobra"
)

var forceFleet bool

var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Create or update a GameLift container fleet",
	Long: `Deploys the container to GameLift. The first run:

  1. Creates a container group definition
  2. Waits for the image to be snapshotted (COPYING -> READY)
  3. Creates the container fleet
  4. Configures inbound UDP permissions. Set gamelift.allowedCidr to a
     restrictive client CIDR for test fleets. When unset, GameLift manages
     the connection port range and opens it to 0.0.0.0/0.

Later runs update the existing fleet in place instead of replacing it: a new
container group definition version is created, then the fleet is rolled onto it
(GameLift validates the new containers on a slice of capacity first, then
replaces the rest, and restores the previous image if validation fails). The
fleet ID stays the same. A re-pushed image tag is detected by its ECR digest;
when nothing changed, nothing is deployed. Use --force to redeploy anyway.

Changing the instance type, containers per instance, or the inbound port range
still requires 'ludus deploy destroy' first.`,
	RunE: runFleet,
}

func init() {
	fleetCmd.Flags().BoolVar(&withSession, "with-session", false, "create a game session after deployment")
	fleetCmd.Flags().BoolVar(&forceFleet, "force", false, "redeploy even if the fleet already runs this image (for example when the digest could not be compared)")
	Cmd.AddCommand(fleetCmd)
}

func runFleet(cmd *cobra.Command, args []string) error {
	if err := config.ValidateContainerIAMPath(globals.Cfg.GameLift.IAM.Path); err != nil {
		return err
	}
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

	fleetStatus, err := deployer.DeployFleet(cmd.Context(), forceFleet)
	if err != nil {
		return diagnose.DeployError(err, "gamelift")
	}

	gamelift.RecordFleetState(fleetStatus, os.Stdout)

	printFleetOutcome(fleetStatus)
	if err := maybeCreateSession(cmd.Context(), gamelift.NewTargetAdapter(deployer)); err != nil {
		return err
	}
	printNextStep()
	return nil
}

// printFleetOutcome reports what DeployFleet did. Inbound rules are only
// stated for a new fleet: an update leaves them as GameLift already has them.
func printFleetOutcome(fleet *gamelift.FleetStatus) {
	switch fleet.Action {
	case gamelift.ActionUnchanged:
		fmt.Printf("\nFleet unchanged: %s (status: %s)\n", fleet.FleetID, fleet.Status)
		return
	case gamelift.ActionUpdated:
		fmt.Printf("\nFleet updated in place: %s (status: %s)\n", fleet.FleetID, fleet.Status)
		return
	}
	fmt.Printf("\nFleet deployed: %s (status: %s)\n", fleet.FleetID, fleet.Status)
	fmt.Printf("Inbound: UDP connection ports open to %s (%s)\n",
		config.ResolvedAllowedCIDR(globals.Cfg.GameLift.AllowedCIDR),
		fleetPortRangeDescription(globals.Cfg.GameLift.AllowedCIDR))
}

// printFleetPlan renders the resolved deploy plan under --dry-run so the
// operator can review every value that would reach AWS before a billable
// create (#778). The tag line surfaces the duplicate-key problem from #775
// before any resource is created.
func printFleetPlan(plan gamelift.FleetPlan) {
	fmt.Println("Dry run — would create the container group definition and fleet, or update an existing fleet in place (decided at deploy time; no AWS calls made).")
	fmt.Printf("  Image:               %s\n", plan.ImageURI)
	fmt.Printf("  Container group:     %s\n", plan.ContainerGroupName)
	fmt.Printf("  Fleet:               %s\n", plan.FleetName)
	boundary := "no boundary"
	if plan.IAMPermissionsBoundary != "" {
		boundary = "boundary " + plan.IAMPermissionsBoundary
	}
	fmt.Printf("  IAM role:            %s (path %s, %s)\n", plan.IAMRoleName, plan.IAMPath, boundary)
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
