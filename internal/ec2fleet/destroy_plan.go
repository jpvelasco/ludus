package ec2fleet

import (
	"fmt"

	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

// PlanDestroy uses the same recorded fleet/build/archive extraction as the
// adapter while leaving fleet discovery and IAM inspection unresolved.
func PlanDestroy(opts DeployOptions, st *state.State) (deploy.DestroyTargetPlan, error) {
	role, err := deploy.PlanIAMRole(iamRoleName, deploy.SourceDefault, opts.resourceTags(), "ec2", "")
	if err != nil {
		return deploy.DestroyTargetPlan{}, err
	}
	recorded := destroyStateFromState(st)
	fleet := deploy.RecordedDestroyItem("fleet", recorded.fleetID, deploy.ActionDelete)
	if fleet.Identifier == "" {
		fleet.Source, fleet.Decision = deploy.SourceDiscovery, deploy.DecisionUnresolved
		fleet.Selector = fmt.Sprintf("EC2 fleet named %q", opts.FleetName)
		fleet.Reason = "Missing recorded ID triggers live name lookup; this does not prove the fleet is absent."
	}
	build := deploy.RecordedDestroyItem("game-build", recorded.buildID, deploy.ActionDelete)
	archive := ""
	if recorded.s3Bucket != "" && recorded.s3Key != "" {
		archive = "s3://" + recorded.s3Bucket + "/" + recorded.s3Key
	}
	object := deploy.RecordedDestroyItem("s3-object", archive, deploy.ActionDelete)
	role.Conditions = append(role.Conditions, "A fleet is found or a build ID is recorded; otherwise the adapter does nothing.")
	items := []deploy.DestroyItem{fleet, build, object, role}
	for i := 1; i < len(items); i++ {
		items[i].Conditions = append(items[i].Conditions, "Fleet deletion succeeds or no fleet ID was resolved.")
		items[i].Reason += " Cleanup failures are reported as best-effort warnings."
	}
	return deploy.DestroyTargetPlan{Target: "ec2", Items: items}, nil
}
