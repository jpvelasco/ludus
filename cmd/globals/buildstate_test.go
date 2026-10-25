package globals

import "testing"

// TestCanSaveBuildState pins the dry-run guard for every build-state save
// (#697, #733): state writes are allowed only when no preview is active.
func TestCanSaveBuildState(t *testing.T) {
	tests := []struct {
		name         string
		effectiveDry bool
		wantCanSave  bool
	}{
		{name: "real build saves state", effectiveDry: false, wantCanSave: true},
		{name: "dry-run preview skips state", effectiveDry: true, wantCanSave: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanSaveBuildState(tt.effectiveDry); got != tt.wantCanSave {
				t.Errorf("CanSaveBuildState(%v) = %v, want %v", tt.effectiveDry, got, tt.wantCanSave)
			}
		})
	}
}
