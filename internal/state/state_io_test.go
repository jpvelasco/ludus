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

func TestLoad_UnreadableState(t *testing.T) {
	setupTest(t)

	// A directory at the state path makes os.ReadFile fail with a
	// non-IsNotExist error, which Load surfaces.
	if err := os.MkdirAll(filepath.Join(stateDir, stateFile), 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected error when state path is a directory")
	}
}

func TestSave_MkdirAllFails(t *testing.T) {
	setupTest(t)

	if err := os.WriteFile(stateDir, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Save(&State{}); err == nil {
		t.Fatal("expected error when stateDir path is occupied by a file")
	}
}

func TestSave_WriteFileFails(t *testing.T) {
	setupTest(t)

	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(stateDir, stateFile), 0755); err != nil {
		t.Fatal(err)
	}

	if err := Save(&State{}); err == nil {
		t.Fatal("expected error when state.json path is occupied by a directory")
	}
}

func TestListProfiles_SortedSkipsDirectories(t *testing.T) {
	setupTest(t)

	profilesDir := filepath.Join(stateDir, "profiles")
	for _, name := range []string{"qa.json", "staging.json", "ignored-dir"} {
		p := filepath.Join(profilesDir, name)
		var err error
		if name == "ignored-dir" {
			err = os.MkdirAll(p, 0755)
		} else {
			if mkErr := os.MkdirAll(filepath.Dir(p), 0755); mkErr != nil {
				t.Fatal(mkErr)
			}
			err = os.WriteFile(p, []byte("{}"), 0644)
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	got := mustListProfiles(t)
	want := []string{"qa", "staging"}
	if len(got) != len(want) {
		t.Fatalf("ListProfiles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListProfiles[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestListProfiles_UnreadableDir(t *testing.T) {
	setupTest(t)

	// A file at the profiles dir path makes os.ReadDir fail; on Windows the
	// error maps to IsNotExist so ListProfiles treats it as "no profiles",
	// which is the intended graceful degradation.
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "profiles"), []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}

	got := mustListProfiles(t)
	if got != nil {
		t.Errorf("ListProfiles = %v, want nil (profiles dir unusable)", got)
	}
}
