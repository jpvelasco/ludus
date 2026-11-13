package stack

import (
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

// RecordedStackName accepts a name only from a recorded stack deployment.
func RecordedStackName(st *state.State) string {
	if st.Deploy == nil || st.Deploy.TargetName != "stack" || st.Fleet == nil {
		return ""
	}
	return st.Fleet.StackName
}

// DefaultStackName is shared by deployment, target resolution, and preview.
func DefaultStackName(fleetName string) string {
	return "ludus-" + fleetName
}

// PlanDestroy describes DeleteStack without guessing its member resources.
func PlanDestroy(fleetName string, st *state.State) deploy.DestroyTargetPlan {
	name, source := RecordedStackName(st), deploy.SourceState
	if name == "" {
		name, source = DefaultStackName(fleetName), deploy.SourceConfig
	}
	return deploy.DestroyTargetPlan{Target: "stack", Items: []deploy.DestroyItem{{
		Kind: "stack", Identifier: name, Source: source,
		Action: deploy.ActionDelete, Decision: deploy.DecisionPlanned,
		Reason: "CloudFormation applies the template and service deletion rules; retention, drift, termination protection, and deletion success are unknown offline.",
	}}}
}
