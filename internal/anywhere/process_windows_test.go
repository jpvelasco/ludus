//go:build windows

package anywhere

import "testing"

func TestWindowsProcessPIDGuards(t *testing.T) {
	for _, tt := range []struct {
		name string
		pid  int
	}{
		{name: "zero", pid: 0},
		{name: "negative", pid: -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := StopServer(tt.pid); err != nil {
				t.Fatalf("StopServer(%d) = %v", tt.pid, err)
			}
			if IsProcessAlive(tt.pid) {
				t.Fatalf("IsProcessAlive(%d) = true, want false", tt.pid)
			}
		})
	}
}

func TestWindowsPositivePIDLivenessIsUnknown(t *testing.T) {
	if IsProcessAlive(1) {
		t.Fatal("IsProcessAlive(1) = true, want false")
	}
}
