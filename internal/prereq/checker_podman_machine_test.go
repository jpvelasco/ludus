package prereq

import (
	"strings"
	"testing"
)

func TestParsePodmanMachineResources_Valid(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantDisk int
		wantMem  int
		wantErr  bool
	}{
		{
			name:     "well-provisioned machine",
			input:    `[{"Resources":{"CPUs":8,"DiskSize":400,"Memory":12288,"USBs":[]}}]`,
			wantDisk: 400,
			wantMem:  12288,
		},
		{
			name:     "under-provisioned defaults",
			input:    `[{"Resources":{"CPUs":4,"DiskSize":100,"Memory":2048,"USBs":[]}}]`,
			wantDisk: 100,
			wantMem:  2048,
		},
		{
			name:     "multiple machines — first is used",
			input:    `[{"Resources":{"DiskSize":400,"Memory":12288}},{"Resources":{"DiskSize":50,"Memory":1024}}]`,
			wantDisk: 400,
			wantMem:  12288,
		},
		{
			name:    "empty array",
			input:   `[]`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   `not json`,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePodmanMachineResources([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr=%v, got err: %v", tt.wantErr, err)
			}
			if !tt.wantErr {
				if got.DiskSize != tt.wantDisk {
					t.Errorf("DiskSize: got %d, want %d", got.DiskSize, tt.wantDisk)
				}
				if got.Memory != tt.wantMem {
					t.Errorf("Memory: got %d, want %d", got.Memory, tt.wantMem)
				}
			}
		})
	}
}

func TestPodmanMachineResourceWarning_WellProvisioned(t *testing.T) {
	// podmanMachineResourceWarning runs a real command — on machines without podman
	// it returns "" (best-effort), which is the correct behavior.
	// We test the warning logic directly via the helper functions instead.
	res := podmanMachineResources{DiskSize: 400, Memory: 12288}
	warn := podmanResourceWarningFromResources(res)
	if warn != "" {
		t.Errorf("expected no warning for well-provisioned machine, got: %s", warn)
	}
}

func TestPodmanMachineResourceWarning_LowDisk(t *testing.T) {
	res := podmanMachineResources{DiskSize: 100, Memory: 12288}
	warn := podmanResourceWarningFromResources(res)
	if warn == "" {
		t.Error("expected warning for low disk, got empty string")
	}
	if !strings.Contains(warn, "disk") {
		t.Errorf("expected 'disk' in warning, got: %s", warn)
	}
	if !strings.Contains(warn, "300") {
		t.Errorf("expected required disk size in warning, got: %s", warn)
	}
}

func TestPodmanMachineResourceWarning_LowMemory(t *testing.T) {
	res := podmanMachineResources{DiskSize: 400, Memory: 2048}
	warn := podmanResourceWarningFromResources(res)
	if warn == "" {
		t.Error("expected warning for low memory, got empty string")
	}
	if !strings.Contains(warn, "memory") {
		t.Errorf("expected 'memory' in warning, got: %s", warn)
	}
}

func TestPodmanMachineResourceWarning_BothLow(t *testing.T) {
	res := podmanMachineResources{DiskSize: 100, Memory: 2048}
	warn := podmanResourceWarningFromResources(res)
	if warn == "" {
		t.Error("expected warning for low disk and memory")
	}
	if !strings.Contains(warn, "disk") || !strings.Contains(warn, "memory") {
		t.Errorf("expected both 'disk' and 'memory' in warning, got: %s", warn)
	}
	if !strings.Contains(warn, "podman machine init") {
		t.Errorf("expected remediation command in warning, got: %s", warn)
	}
}

func TestPodmanMachineResourceWarning_AtThreshold(t *testing.T) {
	res := podmanMachineResources{DiskSize: 300, Memory: 8 * 1024}
	warn := podmanResourceWarningFromResources(res)
	if warn != "" {
		t.Errorf("expected no warning at exact thresholds, got: %s", warn)
	}
}
