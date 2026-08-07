package prereq

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestParseGoMinorVersion(t *testing.T) {
	tests := []struct {
		name                 string
		input                string
		wantMajor, wantMinor int
		wantOK               bool
	}{
		{"standard linux output", "go version go1.25.10 linux/amd64", 1, 25, true},
		{"older 1.18", "go version go1.18 linux/amd64", 1, 18, true},
		{"exactly 1.20", "go version go1.20.0 darwin/arm64", 1, 20, true},
		{"two-component version", "go version go1.21 windows/amd64", 1, 21, true},
		{"trailing newline", "go version go1.23.4 linux/arm64\n", 1, 23, true},
		{"malformed minor component", "go version go1.x linux/amd64", 0, 0, false},
		{"no go token", "some unexpected output", 0, 0, false},
		{"empty", "", 0, 0, false},
		{"goroutine is not a version token", "goroutine running", 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertParseGoMinorVersion(t, tt.input, tt.wantMajor, tt.wantMinor, tt.wantOK)
		})
	}
}

// assertParseGoMinorVersion is a helper to keep TestParseGoMinorVersion short
// and under the 50-LOC complexity limit while preserving full coverage.
func assertParseGoMinorVersion(t *testing.T, input string, wantMajor, wantMinor int, wantOK bool) {
	major, minor, ok := parseGoMinorVersion(input)
	if ok != wantOK {
		t.Fatalf("parseGoMinorVersion(%q) ok = %v, want %v", input, ok, wantOK)
	}
	if !wantOK {
		return
	}
	if major != wantMajor || minor != wantMinor {
		t.Errorf("parseGoMinorVersion(%q) = (%d, %d), want (%d, %d)",
			input, major, minor, wantMajor, wantMinor)
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

func TestCheckGoVersion_GoNotFound(t *testing.T) {
	// Empty PATH makes exec.LookPath("go") fail, exercising the not-found branch.
	t.Setenv("PATH", "")
	c := &Checker{Backend: "docker"}
	result := c.checkGoVersion()
	if result.Passed {
		t.Error("expected fail when go is not on PATH")
	}
	if !strings.Contains(result.Message, "go not found in PATH") {
		t.Errorf("unexpected message: %q", result.Message)
	}
}

func TestCheckGoVersion_ParseFailure(t *testing.T) {
	// A `go` stub that emits unparsable output exercises the warning branch:
	// go ran, but its version could not be determined.
	testsupport.FakeTool(t, "go", testsupport.ToolBehavior{Stdout: "not a go version\n"})
	c := &Checker{Backend: "docker"}
	result := c.checkGoVersion()
	if !result.Passed || !result.Warning {
		t.Errorf("checkGoVersion() = %+v, want pass+warning", result)
	}
	if !strings.Contains(result.Message, "could not determine Go version") {
		t.Errorf("unexpected message: %q", result.Message)
	}
}

func TestCheckGoVersion_TooOldOnPath(t *testing.T) {
	// A `go` stub reporting 1.19 exercises the too-old hard-fail branch.
	testsupport.FakeTool(t, "go", testsupport.ToolBehavior{Stdout: "go version go1.19.3 linux/amd64\n"})
	c := &Checker{Backend: "docker"}
	result := c.checkGoVersion()
	if result.Passed {
		t.Errorf("checkGoVersion() = %+v, want fail for 1.19", result)
	}
	if !strings.Contains(result.Message, "requires Go >= 1.20") {
		t.Errorf("unexpected message: %q", result.Message)
	}
}
