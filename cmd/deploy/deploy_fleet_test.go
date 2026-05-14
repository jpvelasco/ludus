package deploy

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/gamelift"
	"github.com/jpvelasco/ludus/internal/state"
)

func assertFleetState(t *testing.T, s *state.State, wantFleetID, wantStatus string) {
	t.Helper()
	if s.Fleet == nil {
		t.Fatal("expected Fleet to be set")
		return
	}
	if s.Fleet.FleetID != wantFleetID {
		t.Errorf("FleetID = %q, want %q", s.Fleet.FleetID, wantFleetID)
	}
	if s.Fleet.Status != wantStatus {
		t.Errorf("Fleet.Status = %q, want %q", s.Fleet.Status, wantStatus)
	}
	if s.Fleet.CreatedAt == "" {
		t.Error("Fleet.CreatedAt should be set")
	}
	if s.Deploy == nil {
		t.Fatal("expected Deploy to be set")
		return
	}
	if s.Deploy.TargetName != "gamelift" {
		t.Errorf("Deploy.TargetName = %q, want %q", s.Deploy.TargetName, "gamelift")
	}
	if s.Deploy.Status != wantStatus {
		t.Errorf("Deploy.Status = %q, want %q", s.Deploy.Status, wantStatus)
	}
	detail := "fleet " + wantFleetID
	if s.Deploy.Detail != detail {
		t.Errorf("Deploy.Detail = %q, want %q", s.Deploy.Detail, detail)
	}
	if s.Deploy.DeployedAt == "" {
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
	assertFleetState(t, s, "fleet-abc123", "ACTIVE")
}

func TestRecordFleetDeployState_OverwritesPreviousState(t *testing.T) {
	t.Chdir(t.TempDir())

	recordFleetDeployState(&gamelift.FleetStatus{FleetID: "fleet-old", Status: "CREATING"})
	recordFleetDeployState(&gamelift.FleetStatus{FleetID: "fleet-new", Status: "ACTIVE"})

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	assertFleetState(t, s, "fleet-new", "ACTIVE")
}
