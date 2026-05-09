package state

import "testing"

// setupTest changes into a fresh temp dir and resets the active profile.
// Both are automatically restored when the test ends.
func setupTest(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	orig := activeProfile
	t.Cleanup(func() { activeProfile = orig })
	SetProfile("")
}

func mustLoad(t *testing.T) *State {
	t.Helper()
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func mustSave(t *testing.T, s *State) {
	t.Helper()
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func mustListProfiles(t *testing.T) []string {
	t.Helper()
	profiles, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	return profiles
}
