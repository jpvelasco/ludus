package deploy

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/gamelift"
	"github.com/jpvelasco/ludus/internal/state"
)

func TestRecordFleetDeployState(t *testing.T) {
	t.Chdir(t.TempDir())

	fs := &gamelift.FleetStatus{
		FleetID: "fleet-abc123",
		Status:  "ACTIVE",
	}
	recordFleetDeployState(fs)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}

	if s.Fleet == nil {
		t.Fatal("expected Fleet to be set")
	}
	if s.Fleet.FleetID != "fleet-abc123" {
		t.Errorf("FleetID = %q, want %q", s.Fleet.FleetID, "fleet-abc123")
	}
	if s.Fleet.Status != "ACTIVE" {
		t.Errorf("Fleet.Status = %q, want %q", s.Fleet.Status, "ACTIVE")
	}
	if s.Fleet.CreatedAt == "" {
		t.Error("Fleet.CreatedAt should be set")
	}

	if s.Deploy == nil {
		t.Fatal("expected Deploy to be set")
	}
	if s.Deploy.TargetName != "gamelift" {
		t.Errorf("Deploy.TargetName = %q, want %q", s.Deploy.TargetName, "gamelift")
	}
	if s.Deploy.Status != "ACTIVE" {
		t.Errorf("Deploy.Status = %q, want %q", s.Deploy.Status, "ACTIVE")
	}
	if s.Deploy.Detail != "fleet fleet-abc123" {
		t.Errorf("Deploy.Detail = %q, want %q", s.Deploy.Detail, "fleet fleet-abc123")
	}
	if s.Deploy.DeployedAt == "" {
		t.Error("Deploy.DeployedAt should be set")
	}
}

func TestRecordFleetDeployState_OverwritesPreviousState(t *testing.T) {
	t.Chdir(t.TempDir())

	// Record an initial state
	recordFleetDeployState(&gamelift.FleetStatus{FleetID: "fleet-old", Status: "CREATING"})

	// Overwrite with new fleet
	recordFleetDeployState(&gamelift.FleetStatus{FleetID: "fleet-new", Status: "ACTIVE"})

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if s.Fleet == nil {
		t.Fatal("expected Fleet to be set")
	}
	if s.Fleet.FleetID != "fleet-new" {
		t.Errorf("FleetID = %q, want %q (should overwrite old)", s.Fleet.FleetID, "fleet-new")
	}
	if s.Deploy.TargetName != "gamelift" {
		t.Errorf("Deploy.TargetName = %q, want %q", s.Deploy.TargetName, "gamelift")
	}
}
