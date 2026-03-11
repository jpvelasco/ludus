package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatePathForProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    string
	}{
		{"default", "", filepath.Join(".ludus", "state.json")},
		{"named", "staging", filepath.Join(".ludus", "profiles", "staging.json")},
		{"another", "prod", filepath.Join(".ludus", "profiles", "prod.json")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statePathForProfile(tt.profile)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetAndActiveProfile(t *testing.T) {
	// Save and restore the original profile
	orig := activeProfile
	defer func() { activeProfile = orig }()

	SetProfile("test-profile")
	if ActiveProfile() != "test-profile" {
		t.Errorf("got %q, want %q", ActiveProfile(), "test-profile")
	}

	SetProfile("")
	if ActiveProfile() != "" {
		t.Errorf("got %q, want empty", ActiveProfile())
	}
}

func TestLoadSaveRoundtrip(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Save and restore the profile
	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()
	SetProfile("")

	s := &State{
		Fleet: &FleetState{
			FleetID:   "fleet-123",
			Status:    "active",
			CreatedAt: "2025-01-01T00:00:00Z",
		},
		Session: &SessionState{
			SessionID: "session-456",
			IPAddress: "10.0.0.1",
			Port:      7777,
			Status:    "active",
		},
	}

	if err := Save(s); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Fleet == nil {
		t.Fatal("expected fleet state after roundtrip")
	}
	if loaded.Fleet.FleetID != "fleet-123" {
		t.Errorf("fleet ID: got %q, want %q", loaded.Fleet.FleetID, "fleet-123")
	}
	if loaded.Session == nil {
		t.Fatal("expected session state after roundtrip")
	}
	if loaded.Session.IPAddress != "10.0.0.1" {
		t.Errorf("session IP: got %q, want %q", loaded.Session.IPAddress, "10.0.0.1")
	}
	if loaded.Session.Port != 7777 {
		t.Errorf("session port: got %d, want 7777", loaded.Session.Port)
	}
}

func TestLoadMissingFile(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()
	SetProfile("")

	s, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail on missing file: %v", err)
	}
	if s.Fleet != nil || s.Session != nil {
		t.Fatal("expected nil fleet and session for missing file")
	}
}

func TestUpdateAndClearFleet(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()
	SetProfile("")

	if err := UpdateFleet(&FleetState{FleetID: "f-1", Status: "active"}); err != nil {
		t.Fatalf("UpdateFleet: %v", err)
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Fleet == nil || s.Fleet.FleetID != "f-1" {
		t.Fatal("fleet not updated")
	}

	if err := ClearFleet(); err != nil {
		t.Fatalf("ClearFleet: %v", err)
	}

	s, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Fleet != nil {
		t.Error("fleet should be nil after clear")
	}
	if s.Session != nil {
		t.Error("session should also be cleared with fleet")
	}
}

func TestUpdateAndClearSession(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()
	SetProfile("")

	if err := UpdateSession(&SessionState{SessionID: "s-1", IPAddress: "1.2.3.4", Port: 7777}); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Session == nil || s.Session.SessionID != "s-1" {
		t.Fatal("session not updated")
	}

	if err := ClearSession(); err != nil {
		t.Fatalf("ClearSession: %v", err)
	}

	s, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Session != nil {
		t.Error("session should be nil after clear")
	}
}

func TestProfileIsolation(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()

	// Write to default profile
	SetProfile("")
	if err := UpdateFleet(&FleetState{FleetID: "default-fleet"}); err != nil {
		t.Fatal(err)
	}

	// Write to named profile
	SetProfile("staging")
	if err := UpdateFleet(&FleetState{FleetID: "staging-fleet"}); err != nil {
		t.Fatal(err)
	}

	// Read back default — should have default-fleet
	SetProfile("")
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Fleet == nil || s.Fleet.FleetID != "default-fleet" {
		t.Errorf("default profile: got fleet %v, want default-fleet", s.Fleet)
	}

	// Read back staging — should have staging-fleet
	SetProfile("staging")
	s, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Fleet == nil || s.Fleet.FleetID != "staging-fleet" {
		t.Errorf("staging profile: got fleet %v, want staging-fleet", s.Fleet)
	}
}

func TestListProfiles(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()

	// No profiles dir yet
	profiles, err := ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}

	// Create profiles by writing to them
	SetProfile("beta")
	if err := Save(&State{}); err != nil {
		t.Fatal(err)
	}
	SetProfile("alpha")
	if err := Save(&State{}); err != nil {
		t.Fatal(err)
	}

	profiles, err = ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
	// Should be sorted
	if profiles[0] != "alpha" || profiles[1] != "beta" {
		t.Errorf("expected [alpha beta], got %v", profiles)
	}
}

func TestDeleteProfile(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()

	// Cannot delete default
	if err := DeleteProfile(""); err == nil {
		t.Error("expected error deleting default profile")
	}

	// Cannot delete non-existent
	if err := DeleteProfile("ghost"); err == nil {
		t.Error("expected error deleting non-existent profile")
	}

	// Create and delete
	SetProfile("temp")
	if err := Save(&State{}); err != nil {
		t.Fatal(err)
	}

	if err := DeleteProfile("temp"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}

	profiles, err := ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range profiles {
		if p == "temp" {
			t.Error("profile 'temp' should have been deleted")
		}
	}
}

func TestLoadCorruptedStateFile(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()
	SetProfile("")

	// Write corrupted JSON
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, stateFile), []byte("{corrupt!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = Load()
	if err == nil {
		t.Fatal("expected error loading corrupted state file")
	}
}

func TestLoadEmptyJSONState(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()
	SetProfile("")

	// Write valid but empty JSON object
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, stateFile), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if s.Fleet != nil || s.Session != nil || s.Deploy != nil {
		t.Error("expected all nil fields for empty JSON state")
	}
}

func TestUpdateDeploy(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()
	SetProfile("")

	if err := UpdateDeploy(&DeployState{
		TargetName: "gamelift",
		Status:     "active",
		Detail:     "fleet-abc",
	}); err != nil {
		t.Fatal(err)
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Deploy == nil {
		t.Fatal("expected deploy state")
	}
	if s.Deploy.TargetName != "gamelift" {
		t.Errorf("target name: got %q, want %q", s.Deploy.TargetName, "gamelift")
	}
}

func TestUpdateEC2Fleet(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()
	SetProfile("")

	if err := UpdateEC2Fleet(&EC2FleetState{
		FleetID:  "ec2-fleet-1",
		BuildID:  "build-1",
		S3Bucket: "my-bucket",
		Status:   "active",
	}); err != nil {
		t.Fatal(err)
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.EC2Fleet == nil || s.EC2Fleet.FleetID != "ec2-fleet-1" {
		t.Fatal("EC2 fleet not updated")
	}

	if err := ClearEC2Fleet(); err != nil {
		t.Fatal(err)
	}
	s, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.EC2Fleet != nil {
		t.Error("EC2 fleet should be nil after clear")
	}
}

func TestUpdateAnywhere(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()
	SetProfile("")

	if err := UpdateAnywhere(&AnywhereState{
		FleetID:    "anywhere-1",
		IPAddress:  "192.168.1.1",
		ServerPort: 7777,
	}); err != nil {
		t.Fatal(err)
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Anywhere == nil || s.Anywhere.FleetID != "anywhere-1" {
		t.Fatal("anywhere not updated")
	}

	if err := ClearAnywhere(); err != nil {
		t.Fatal(err)
	}
	s, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Anywhere != nil {
		t.Error("anywhere should be nil after clear")
	}
}

func TestUpdateEngineImage(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()
	SetProfile("")

	if err := UpdateEngineImage(&EngineImageState{
		ImageTag: "ludus-engine:5.7",
		Version:  "5.7",
		BuiltAt:  "2025-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.EngineImage == nil || s.EngineImage.ImageTag != "ludus-engine:5.7" {
		t.Fatal("engine image not updated")
	}
}

func TestUpdateClient(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	origProfile := activeProfile
	defer func() { activeProfile = origProfile }()
	SetProfile("")

	if err := UpdateClient(&ClientState{
		BinaryPath: "/path/to/client",
		Platform:   "Win64",
		BuiltAt:    "2025-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Client == nil || s.Client.Platform != "Win64" {
		t.Fatal("client not updated")
	}
}
