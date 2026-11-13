package gamelift

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/state"
)

func TestDestroyPlanUsesOnlyRecordedContainerFleet(t *testing.T) {
	for _, tt := range []struct{ target, id, want string }{
		{"gamelift", "recorded-fleet", "recorded-fleet"},
		{"stack", "foreign-fleet", ""},
		{"", "", ""},
	} {
		t.Run(tt.target, func(t *testing.T) {
			opts := newTestFleetDeployer(nil, nil).opts
			opts.OwnershipTagKey = "Deployment"
			st := &state.State{Deploy: &state.DeployState{TargetName: tt.target}, Fleet: &state.FleetState{FleetID: tt.id}}
			plan, err := PlanDestroy(opts, st)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Items[0].Identifier != tt.want {
				t.Fatalf("wrong fleet selected: %+v", plan.Items[0])
			}
			assertContainerPlanGuards(t, plan.Items[1].Decision, plan.Items[2].Decision, strings.Join(plan.Items[2].Conditions, "\n"))
		})
	}
}

func assertContainerPlanGuards(t *testing.T, groupDecision, roleDecision, conditions string) {
	t.Helper()
	if groupDecision != "conditional" || roleDecision != "conditional" {
		t.Fatalf("live guards were decided offline: group=%s role=%s", groupDecision, roleDecision)
	}
	for _, want := range []string{"CloudFormation", "Project=\"test-project\"", "Deployment=\"test-fleet\"", "absent or equal"} {
		if !strings.Contains(conditions, want) {
			t.Errorf("missing %q in %s", want, conditions)
		}
	}
}

func TestDestroyPlanRoleNameAndInvalidInputs(t *testing.T) {
	opts := newTestFleetDeployer(nil, nil).opts
	opts.IAMRoleName = "custom-role"
	plan, err := PlanDestroy(opts, &state.State{})
	if err != nil {
		t.Fatal(err)
	}
	if role := plan.Items[2]; role.Identifier != "custom-role" || role.Source != "config" {
		t.Fatalf("role = %+v", role)
	}
	opts.ContainerGroupName = ""
	if _, err := PlanDestroy(opts, &state.State{}); err == nil {
		t.Fatal("unknown group accepted")
	}
	opts.Tags = nil
	if _, err := PlanDestroy(opts, &state.State{}); err == nil {
		t.Fatal("invalid role identity accepted")
	}
}
