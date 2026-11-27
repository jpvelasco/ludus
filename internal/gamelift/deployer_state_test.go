package gamelift

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

const seededTime = "2020-01-02T03:04:05Z"

// seedFleetState records fleet-1 as this profile's GameLift deployment.
func seedFleetState(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := state.UpdateFleet(&state.FleetState{FleetID: "fleet-1", Status: "ACTIVE", CreatedAt: seededTime}); err != nil {
		t.Fatalf("seed fleet state: %v", err)
	}
	if err := state.UpdateDeploy(&state.DeployState{TargetName: "gamelift", Status: "ACTIVE", Detail: "fleet fleet-1", DeployedAt: seededTime}); err != nil {
		t.Fatalf("seed deploy state: %v", err)
	}
}

func loadState(t *testing.T) *state.State {
	t.Helper()
	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load(): %v", err)
	}
	return s
}

func TestRecordFleetStateCreatedStampsNewTimes(t *testing.T) {
	seedFleetState(t)

	RecordFleetState(&FleetStatus{FleetID: "fleet-2", Status: "ACTIVE", Action: ActionCreated}, &bytes.Buffer{})

	s := loadState(t)
	if s.Fleet.FleetID != "fleet-2" || s.Fleet.CreatedAt == seededTime || s.Deploy.DeployedAt == seededTime {
		t.Errorf("state after create = fleet %+v deploy %+v, want fleet-2 with fresh times", s.Fleet, s.Deploy)
	}
}

func TestRecordFleetStateUpdateKeepsCreatedAt(t *testing.T) {
	seedFleetState(t)

	RecordFleetState(&FleetStatus{FleetID: "fleet-1", Status: "ACTIVE", Action: ActionUpdated}, &bytes.Buffer{})

	s := loadState(t)
	if s.Fleet.CreatedAt != seededTime {
		t.Errorf("Fleet.CreatedAt = %q, want the original %q", s.Fleet.CreatedAt, seededTime)
	}
	if s.Deploy.DeployedAt == seededTime {
		t.Error("Deploy.DeployedAt was not refreshed by an update")
	}
}

func TestRecordFleetStateUnchangedLeavesRecordedFleetAlone(t *testing.T) {
	seedFleetState(t)

	RecordFleetState(&FleetStatus{FleetID: "fleet-1", Status: "ACTIVE", Action: ActionUnchanged}, &bytes.Buffer{})

	if s := loadState(t); s.Deploy.DeployedAt != seededTime {
		t.Errorf("Deploy.DeployedAt = %q, want %q untouched by a no-op deploy", s.Deploy.DeployedAt, seededTime)
	}
}

func TestRecordFleetStateUnchangedAdoptsAnUnrecordedFleet(t *testing.T) {
	t.Chdir(t.TempDir())

	RecordFleetState(&FleetStatus{FleetID: "fleet-9", Status: "ACTIVE", Action: ActionUnchanged}, &bytes.Buffer{})

	if s := loadState(t); s.Fleet == nil || s.Fleet.FleetID != "fleet-9" {
		t.Errorf("fleet state = %+v, want fleet-9 recorded so later commands find it", s.Fleet)
	}
}

func TestRecordFleetStateWarnsOnTheWriterWhenStateIsUnwritable(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, ".ludus"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("create .ludus blocker: %v", err)
	}
	var out bytes.Buffer

	RecordFleetState(&FleetStatus{FleetID: "fleet-1", Status: "ACTIVE", Action: ActionCreated}, &out)

	if !strings.Contains(out.String(), "failed to write fleet state") || !strings.Contains(out.String(), "failed to write deploy state") {
		t.Errorf("warnings = %q, want both write failures reported", out.String())
	}
}

func TestAdapterDeployReportsAnInPlaceUpdate(t *testing.T) {
	probe := chooseFixture()
	running, next := scenarioDefs(probe, "sha256:old", "sha256:new")
	s := newUpdateScenario(t, running, running, next, "sha256:new")

	result, err := NewTargetAdapter(s.d).Deploy(context.Background(), deploy.DeployInput{})
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if result.Detail != "fleet fleet-1 (updated)" {
		t.Errorf("Deploy() detail = %q, want the update reported", result.Detail)
	}
	if got := loadState(t); got.Fleet == nil || got.Fleet.FleetID != "fleet-1" {
		t.Errorf("fleet state = %+v, want fleet-1 recorded", got.Fleet)
	}
}
