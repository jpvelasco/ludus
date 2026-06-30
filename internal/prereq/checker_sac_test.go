//go:build windows

package prereq

import "testing"

func TestInterpretSACState(t *testing.T) {
	tests := []struct {
		name       string
		state      string
		wantActive bool
		wantMode   string
	}{
		{"empty property absent", "", false, ""},
		{"missing key sentinel", "missing", false, ""},
		{"explicit off", "0", false, ""},
		{"enforce", "1", true, "enforcement"},
		{"evaluation", "2", true, "evaluation"},
		{"unexpected value", "99", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active, mode := interpretSACState(tt.state)
			if active != tt.wantActive || mode != tt.wantMode {
				t.Errorf("interpretSACState(%q) = (%v, %q); want (%v, %q)",
					tt.state, active, mode, tt.wantActive, tt.wantMode)
			}
		})
	}
}
