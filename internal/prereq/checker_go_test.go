package prereq

import (
	"testing"
)

func TestParseGoMinorVersion(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMajor int
		wantMinor int
		wantOK    bool
	}{
		{
			name:      "standard linux output",
			input:     "go version go1.25.10 linux/amd64",
			wantMajor: 1, wantMinor: 25, wantOK: true,
		},
		{
			name:      "older 1.18",
			input:     "go version go1.18 linux/amd64",
			wantMajor: 1, wantMinor: 18, wantOK: true,
		},
		{
			name:      "exactly 1.20",
			input:     "go version go1.20.0 darwin/arm64",
			wantMajor: 1, wantMinor: 20, wantOK: true,
		},
		{
			name:      "two-component version",
			input:     "go version go1.21 windows/amd64",
			wantMajor: 1, wantMinor: 21, wantOK: true,
		},
		{
			name:      "trailing newline",
			input:     "go version go1.23.4 linux/arm64\n",
			wantMajor: 1, wantMinor: 23, wantOK: true,
		},
		{
			name:   "no go token",
			input:  "some unexpected output",
			wantOK: false,
		},
		{
			name:   "empty",
			input:  "",
			wantOK: false,
		},
		{
			name:   "goroutine is not a version token",
			input:  "goroutine running",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, ok := parseGoMinorVersion(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseGoMinorVersion(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if major != tt.wantMajor || minor != tt.wantMinor {
				t.Errorf("parseGoMinorVersion(%q) = (%d, %d), want (%d, %d)",
					tt.input, major, minor, tt.wantMajor, tt.wantMinor)
			}
		})
	}
}

func TestCheckGoVersion_NonContainerBackendSkips(t *testing.T) {
	// Explicit non-container backends skip the Go check (wrapper build not involved).
	for _, backend := range []string{"native", "wsl2"} {
		c := &Checker{Backend: backend}
		result := c.checkGoVersion()
		if !result.Passed {
			t.Errorf("backend %q: expected pass (skip), got fail: %s", backend, result.Message)
		}
		if result.Warning {
			t.Errorf("backend %q: expected non-warning skip, got warning: %s", backend, result.Message)
		}
	}
}

func TestCheckGoVersion_ContainerBackendChecks(t *testing.T) {
	// With a container backend (or default "") the check probes host `go`.
	// Default "" must *not* skip — this covers `ludus container build` with
	// no --backend when config backend is native/wsl2 (Resolve returns "").
	// Must pass on this host (Go 1.25+).
	for _, be := range []string{"docker", "", "podman"} {
		c := &Checker{Backend: be}
		result := c.checkGoVersion()
		if result.Name != "Go compiler version" {
			t.Errorf("backend %q: unexpected check name: %q", be, result.Name)
		}
		if !result.Passed {
			t.Errorf("backend %q: expected pass on this host, got fail: %s", be, result.Message)
		}
	}
}
