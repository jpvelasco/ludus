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
	orig := activeProfile
	defer func() { activeProfile = orig }()

	SetProfile("test-profile")
	if got := ActiveProfile(); got != "test-profile" {
		t.Errorf("got %q, want %q", got, "test-profile")
	}

	SetProfile("")
	if got := ActiveProfile(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestProfileIsolation(t *testing.T) {
	setupTest(t)

	if err := UpdateFleet(&FleetState{FleetID: "default-fleet"}); err != nil {
		t.Fatal(err)
	}

	SetProfile("staging")
	if err := UpdateFleet(&FleetState{FleetID: "staging-fleet"}); err != nil {
		t.Fatal(err)
	}

	SetProfile("")
	s := mustLoad(t)
	if s.Fleet == nil || s.Fleet.FleetID != "default-fleet" {
		t.Errorf("default profile: got fleet %v, want default-fleet", s.Fleet)
	}

	SetProfile("staging")
	s = mustLoad(t)
	if s.Fleet == nil || s.Fleet.FleetID != "staging-fleet" {
		t.Errorf("staging profile: got fleet %v, want staging-fleet", s.Fleet)
	}
}

func TestListProfiles(t *testing.T) {
	setupTest(t)

	if profiles := mustListProfiles(t); len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}

	SetProfile("beta")
	mustSave(t, &State{})
	SetProfile("alpha")
	mustSave(t, &State{})

	profiles := mustListProfiles(t)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
	if profiles[0] != "alpha" || profiles[1] != "beta" {
		t.Errorf("expected [alpha beta], got %v", profiles)
	}
}

func TestDeleteProfile(t *testing.T) {
	t.Run("rejects default profile", func(t *testing.T) {
		if err := DeleteProfile(""); err == nil {
			t.Error("expected error deleting default profile")
		}
	})

	t.Run("rejects non-existent profile", func(t *testing.T) {
		setupTest(t)
		if err := DeleteProfile("ghost"); err == nil {
			t.Error("expected error deleting non-existent profile")
		}
	})

	t.Run("deletes existing profile", func(t *testing.T) {
		setupTest(t)

		SetProfile("temp")
		mustSave(t, &State{})

		if err := DeleteProfile("temp"); err != nil {
			t.Fatalf("DeleteProfile: %v", err)
		}

		for _, p := range mustListProfiles(t) {
			if p == "temp" {
				t.Error("profile 'temp' should have been deleted")
			}
		}
	})
}

// TestValidateProfileName pins the profile-name grammar: a name maps to
// exactly one file directly beneath .ludus/profiles, so separators,
// parent-directory segments, and absolute-path markers must be rejected.
// Empty is valid and selects the default profile.
func TestValidateProfileName(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		wantErr bool
	}{
		{"empty means default", "", false},
		{"simple", "staging", false},
		{"hyphenated", "ue57-ec2", false},
		{"dot separator", "prod.eu", false},
		{"underscore", "dev_2", false},
		{"single dot", ".", true},
		{"double dot", "..", true},
		{"parent traversal", "../state", true},
		{"deep traversal", "a/../../b", true},
		{"forward slash", "a/b", true},
		{"backslash", `a\b`, true},
		{"trailing slash", "staging/", true},
		{"windows drive", "C:temp", true},
		{"leading dot hidden", ".hidden", true},
		{"space", "my profile", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProfileName(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProfileName(%q) error = %v, wantErr %v", tt.profile, err, tt.wantErr)
			}
		})
	}
}

// TestDeleteProfileRejectsTraversal proves an unsafe name cannot escape the
// profiles directory: the decoy is the DEFAULT state file, which the legacy
// path derivation would resolve to and delete.
func TestDeleteProfileRejectsTraversal(t *testing.T) {
	setupTest(t)

	if err := os.MkdirAll(filepath.Join(stateDir, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	decoy := filepath.Join(stateDir, "state.json")
	if err := os.WriteFile(decoy, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := DeleteProfile("../state"); err == nil {
		t.Fatal(`DeleteProfile("../state") = nil, want validation error`)
	}
	if _, err := os.Stat(decoy); err != nil {
		t.Fatalf("default state.json deleted via profile traversal: %v", err)
	}
}
