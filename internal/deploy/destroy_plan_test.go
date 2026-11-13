package deploy

import (
	"reflect"
	"strings"
	"testing"
)

func TestTargetNamesPreservesOrderAcrossCallers(t *testing.T) {
	want := []string{"gamelift", "stack", "ec2", "anywhere", "binary"}
	names := TargetNames()
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("sweep order = %v, want %v", names, want)
	}
	names[0] = "changed-by-caller"
	if !reflect.DeepEqual(TargetNames(), want) {
		t.Fatal("one caller changed the order for subsequent sweeps")
	}
}

func TestRenderDestroyPlanPreservesScopeAndConditions(t *testing.T) {
	plan := &DestroyPlan{
		Version: 1, Profile: "preview", ConfiguredRegion: "us-east-1", ConfiguredAccountID: "123456789012",
		Targets: []DestroyTargetPlan{{Target: "gamelift", Items: []DestroyItem{{
			Kind: "fleet", Action: ActionDelete, Source: SourceDiscovery, Decision: DecisionUnresolved,
			Selector: "configured group", Conditions: []string{"unique match"}, Reason: "live lookup required",
		}}}},
		Purge:    []DestroyItem{{Kind: "ecr-repository", Identifier: "server", Action: ActionDelete, Source: SourceConfig, Decision: DecisionPlanned}},
		Warnings: []string{"example warning"},
	}
	text := RenderDestroyPlan(plan)
	for _, want := range []string{"no AWS calls or changes made", "Profile: preview", "us-east-1", "123456789012", "[unresolved; discovery]", "configured group", "unique match", "live lookup required", "without an ownership tag guard", "example warning"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %s", want, text)
		}
	}
}

func TestRenderDestroyPlanUnconfiguredScope(t *testing.T) {
	text := RenderDestroyPlan(&DestroyPlan{})
	if strings.Count(text, "resolved during execution") != 2 || !strings.Contains(text, "Profile: default") {
		t.Fatalf("unconfigured scope = %s", text)
	}
}

func TestRecordedDestroyItem(t *testing.T) {
	for _, tt := range []struct{ id, decision string }{{"", "skip"}, {"recorded-id", "planned"}} {
		item := RecordedDestroyItem("fleet", tt.id, ActionDelete)
		if item.Identifier != tt.id || item.Decision != tt.decision || item.Source != SourceState {
			t.Fatalf("recorded item = %+v", item)
		}
	}
}

func TestPlanIAMRoleRequiresAValidIdentity(t *testing.T) {
	if _, err := PlanIAMRole("role", SourceConfig, nil, "gamelift", ""); err == nil {
		t.Fatal("invalid identity accepted")
	}
	expected := map[string]string{"ManagedBy": "team", "Project": "game", "ludus:fleet-name": "fleet"}
	item, err := PlanIAMRole("role", SourceDefault, expected, "gamelift", "")
	if err != nil {
		t.Fatal(err)
	}
	if item.Decision != DecisionConditional || !strings.Contains(strings.Join(item.Conditions, "\n"), "Project=\"game\"") {
		t.Fatalf("role plan = %+v", item)
	}
}
