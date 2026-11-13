package gamelift

import (
	"fmt"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

// PlanDestroy resolves local container-fleet candidates and preserves the
// ownership and shared-reference decisions for execution.
func PlanDestroy(opts DeployOptions, st *state.State) (deploy.DestroyTargetPlan, error) {
	source := deploy.SourceConfig
	if opts.IAMRoleName == "" {
		source = deploy.SourceDefault
	}
	role, err := deploy.PlanIAMRole(opts.resolvedRoleName(), source, opts.resourceTags(), "gamelift", opts.OwnershipTagKey)
	if err != nil {
		return deploy.DestroyTargetPlan{}, err
	}
	if opts.ContainerGroupName == "" {
		return deploy.DestroyTargetPlan{}, fmt.Errorf("container group name is required to plan gamelift destruction")
	}
	role.Conditions = append(role.Conditions, "Earlier teardown steps complete without a fatal error.")
	role.Reason += " The original role/group names are not recorded; current configuration supplies them."
	fleet := deploy.RecordedDestroyItem("fleet", recordedFleetIDFromState(st), deploy.ActionDelete)
	if fleet.Identifier == "" {
		fleet.Decision, fleet.Source = deploy.DecisionUnresolved, deploy.SourceDiscovery
		fleet.Selector = fmt.Sprintf("container group %q; %s=%q, with legacy tag and unique-fleet fallback",
			opts.ContainerGroupName, config.ResolvedOwnershipTagKey(opts.OwnershipTagKey), opts.FleetName)
		fleet.Reason = "No fleet ID recorded for a gamelift deployment; live discovery must select a unique candidate."
	}
	group := deploy.DestroyItem{
		Kind: "container-group", Identifier: opts.ContainerGroupName, Source: deploy.SourceConfig,
		Action: deploy.ActionDelete, Decision: deploy.DecisionConditional,
		Conditions: []string{"Fleet teardown succeeds.", "No other fleets reference the group; retain it if reference inspection fails."},
		Reason:     "Current configured name; state cannot reconstruct a previous group name.",
	}
	return deploy.DestroyTargetPlan{Target: "gamelift", Items: []deploy.DestroyItem{fleet, group, role}}, nil
}
