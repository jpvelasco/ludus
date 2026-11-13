package stack

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/state"
)

func TestDestroyPlanStackNameSelection(t *testing.T) {
	for _, tt := range []struct {
		name string
		st   state.State
		want string
	}{
		{"recorded stack", state.State{Deploy: &state.DeployState{TargetName: "stack"}, Fleet: &state.FleetState{StackName: "custom-stack"}}, "custom-stack"},
		{"another target", state.State{Deploy: &state.DeployState{TargetName: "gamelift"}, Fleet: &state.FleetState{StackName: "foreign"}}, "ludus-example"},
		{"empty", state.State{}, "ludus-example"},
		{"no fleet", state.State{Deploy: &state.DeployState{TargetName: "stack"}}, "ludus-example"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			plan := PlanDestroy("example", &tt.st)
			if len(plan.Items) != 1 || plan.Items[0].Identifier != tt.want {
				t.Fatalf("stack plan = %+v", plan)
			}
		})
	}
}
