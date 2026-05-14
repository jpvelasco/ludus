package deploy

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/gamelift"
	"github.com/jpvelasco/ludus/internal/state"
)

func assertFleet(t *testing.T, fleet *state.FleetState, wantFleetID, wantStatus string) {
	t.Helper()
	if fleet == nil {
		t.Fatal("expected Fleet to be set")
		return
	}
	if fleet.FleetID != wantFleetID {
		t.Errorf("FleetID = %q, want %q", fleet.FleetID, wantFleetID)
	}
	if fleet.Status != wantStatus {
		t.Errorf("Fleet.Status = %q, want %q", fleet.Status, wantStatus)
	}
	if fleet.CreatedAt == "" {
		t.Error("Fleet.CreatedAt should be set")
	}
}

func assertDeploy(t *testing.T, deploy *state.DeployState, wantFleetID, wantStatus string) {
	t.Helper()
	if deploy == nil {
		t.Fatal("expected Deploy to be set")
		return
	}
	if deploy.TargetName != "gamelift" {
		t.Errorf("Deploy.TargetName = %q, want %q", deploy.TargetName, "gamelift")
	}
	if deploy.Status != wantStatus {
		t.Errorf("Deploy.Status = %q, want %q", deploy.Status, wantStatus)
	}
	detail := "fleet " + wantFleetID
	if deploy.Detail != detail {
		t.Errorf("Deploy.Detail = %q, want %q", deploy.Detail, detail)
	}
	if deploy.DeployedAt == "" {
		t.Error("Deploy.DeployedAt should be set")
	}
}

func TestRecordFleetDeployState(t *testing.T) {
	t.Chdir(t.TempDir())

	recordFleetDeployState(&gamelift.FleetStatus{FleetID: "fleet-abc123", Status: "ACTIVE"})

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	assertFleet(t, s.Fleet, "fleet-abc123", "ACTIVE")
	assertDeploy(t, s.Deploy, "fleet-abc123", "ACTIVE")
}

func TestRecordFleetDeployState_OverwritesPreviousState(t *testing.T) {
	t.Chdir(t.TempDir())

	recordFleetDeployState(&gamelift.FleetStatus{FleetID: "fleet-old", Status: "CREATING"})
	recordFleetDeployState(&gamelift.FleetStatus{FleetID: "fleet-new", Status: "ACTIVE"})

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	assertFleet(t, s.Fleet, "fleet-new", "ACTIVE")
	assertDeploy(t, s.Deploy, "fleet-new", "ACTIVE")
}
