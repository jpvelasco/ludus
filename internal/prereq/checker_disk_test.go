package prereq

import (
	"strings"
	"testing"
)

func TestDiskSpaceResult(t *testing.T) {
	tests := []struct {
		name       string
		freeGB     uint64
		backend    string
		wantPassed bool
		wantNeedGB int
	}{
		{
			name:       "native build with ample disk passes",
			freeGB:     400,
			backend:    "",
			wantPassed: true,
			wantNeedGB: nativeDiskRequiredGB,
		},
		{
			name:       "native build below 300 fails",
			freeGB:     250,
			backend:    "native",
			wantPassed: false,
			wantNeedGB: nativeDiskRequiredGB,
		},
		{
			name:       "container build with 300-999 GB fails (needs 1 TB)",
			freeGB:     500,
			backend:    "docker",
			wantPassed: false,
			wantNeedGB: containerDiskRequiredGB,
		},
		{
			name:       "container build above 1 TB passes",
			freeGB:     1100,
			backend:    "podman",
			wantPassed: true,
			wantNeedGB: containerDiskRequiredGB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := diskSpaceResult(tt.freeGB, tt.backend)
			if res.Name != "Disk Space" {
				t.Errorf("Name = %q, want \"Disk Space\"", res.Name)
			}
			if res.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v (msg: %s)", res.Passed, tt.wantPassed, res.Message)
			}
			if !tt.wantPassed && !strings.Contains(res.Message, "need") {
				t.Errorf("failing result should state requirement, got: %s", res.Message)
			}
		})
	}
}
