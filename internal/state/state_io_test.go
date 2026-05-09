package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundtrip(t *testing.T) {
	setupTest(t)

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
	mustSave(t, s)

	loaded := mustLoad(t)

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
	setupTest(t)

	s := mustLoad(t)
	if s.Fleet != nil || s.Session != nil {
		t.Fatal("expected nil fleet and session for missing file")
	}
}

func TestLoadCorrupted(t *testing.T) {
	setupTest(t)

	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, stateFile), []byte("{corrupt!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected error loading corrupted state file")
	}
}

func TestLoadEmptyJSON(t *testing.T) {
	setupTest(t)

	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, stateFile), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	s := mustLoad(t)
	if s.Fleet != nil || s.Session != nil || s.Deploy != nil {
		t.Error("expected all nil fields for empty JSON state")
	}
}
