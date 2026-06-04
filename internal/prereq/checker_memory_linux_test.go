//go:build linux

package prereq

import (
	"testing"
)

func TestParseMemTotalKB_Valid(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantGB  uint64
		wantErr bool
	}{
		{
			name:   "32 GB",
			input:  "MemTotal:       33554432 kB\nMemFree:        1048576 kB\n",
			wantGB: 32,
		},
		{
			name:   "16 GB exact",
			input:  "MemTotal:       16777216 kB\n",
			wantGB: 16,
		},
		{
			name:   "MemTotal not first line",
			input:  "SomeOther: 123 kB\nMemTotal: 33554432 kB\n",
			wantGB: 32,
		},
		{
			name:    "missing MemTotal",
			input:   "MemFree: 1048576 kB\nBuffers: 512 kB\n",
			wantErr: true,
		},
		{
			name:    "malformed MemTotal — no value",
			input:   "MemTotal:\n",
			wantErr: true,
		},
		{
			name:    "non-numeric value",
			input:   "MemTotal: invalid kB\n",
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMemTotalKB([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr=%v, got err: %v", tt.wantErr, err)
			}
			if !tt.wantErr && got != tt.wantGB {
				t.Errorf("got %d GB, want %d GB", got, tt.wantGB)
			}
		})
	}
}

func TestCheckMemory_LiveSystem(t *testing.T) {
	c := &Checker{}
	result := c.checkMemory()
	if result.Name != "Memory" {
		t.Errorf("expected name 'Memory', got: %s", result.Name)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
	// /proc/meminfo always exists on Linux.
	if result.Message == "cannot read /proc/meminfo: open /proc/meminfo: no such file or directory" {
		t.Error("/proc/meminfo unexpectedly missing on Linux")
	}
}
