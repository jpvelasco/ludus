package anywhere

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/internal/state"
)

func TestDestroyPlanAnywhereRecordedCandidates(t *testing.T) {
	setTestHome(t, t.TempDir())
	st := &state.State{Anywhere: &state.AnywhereState{
		PID: 2147483647, FleetID: "fleet", ComputeName: "compute",
		LocationName: "custom-location", WrapperConfigDir: "recorded-runtime",
	}}
	plan := PlanDestroy(st)
	for i, want := range []string{"2147483647", "compute", "fleet", "custom-location", "recorded-runtime"} {
		if plan.Items[i].Identifier != want || plan.Items[i].Decision != "planned" {
			t.Errorf("item %d = %+v, want %s", i, plan.Items[i], want)
		}
	}
}

func TestDestroyPlanAnywhereMissingComponents(t *testing.T) {
	setTestHome(t, t.TempDir())
	for _, st := range []*state.State{{}, {Anywhere: &state.AnywhereState{}}} {
		plan := PlanDestroy(st)
		for _, item := range plan.Items {
			if item.Decision != "skip" {
				t.Errorf("missing state produced %+v", item)
			}
		}
	}
}

func TestDestroyPlanAnywhereLegacyDirectoryRequiresConfig(t *testing.T) {
	testHome := t.TempDir()
	setTestHome(t, testHome)
	legacy := filepath.Join(testHome, ".cache", "ludus", "anywhere")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(legacy, wrapperConfigFileName)
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := PlanDestroy(&state.State{Anywhere: &state.AnywhereState{}})
	if item := plan.Items[4]; item.Identifier != legacy || item.Source != "default" {
		t.Fatalf("legacy plan = %+v", item)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "keep" {
		t.Fatalf("preview modified legacy config: %q, %v", data, err)
	}
}
