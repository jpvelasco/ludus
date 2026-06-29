package prereq

import (
	"strings"
	"testing"
)

// When a prebuilt engine image is configured, the container build does not read
// the host engine source tree, so the engine-source and toolchain checks must
// pass (skip) rather than demand a Setup.sh / cross-toolchain that isn't there.
// Regression test for #394 (ludus run validate stage).

func TestCheckEngineSource_PrebuiltImageSkips(t *testing.T) {
	// PrebuiltImage set, EngineSourcePath empty (would otherwise fail).
	res := (&Checker{PrebuiltImage: true}).checkEngineSource()
	if !res.Passed {
		t.Fatalf("expected pass with prebuilt image, got fail: %s", res.Message)
	}
	if !strings.Contains(res.Message, "prebuilt") {
		t.Errorf("expected prebuilt-skip message, got %q", res.Message)
	}
}

func TestCheckToolchain_PrebuiltImageSkips(t *testing.T) {
	res := (&Checker{PrebuiltImage: true}).checkToolchain()
	if !res.Passed {
		t.Fatalf("expected pass with prebuilt image, got fail: %s", res.Message)
	}
	if res.Warning {
		t.Errorf("expected non-warning skip with prebuilt image, got warning: %s", res.Message)
	}
}

func TestRunAll_PrebuiltImageDoesNotFailOnMissingEngineSource(t *testing.T) {
	// Reproduces #394: with a prebuilt engine image and no host engine source,
	// the Engine Source, Toolchain, and Disk Space checks must not fail.
	c := NewChecker("", "", false, nil)
	c.PrebuiltImage = true
	c.Backend = "docker"

	guarded := map[string]bool{"Engine Source": true, "Toolchain": true, "Disk Space": true}
	for _, r := range c.RunAll() {
		if guarded[r.Name] && !r.Passed {
			t.Errorf("%s should not fail with prebuilt image: %s", r.Name, r.Message)
		}
	}
}

func TestDiskCheckPath(t *testing.T) {
	tests := []struct {
		name     string
		checker  Checker
		wantDot  bool
		wantPath string
	}{
		{"prebuilt image ignores engine path", Checker{PrebuiltImage: true, EngineSourcePath: "/some/engine"}, true, ""},
		{"empty engine path falls back", Checker{}, true, ""},
		{"engine path used when no prebuilt", Checker{EngineSourcePath: "/some/engine"}, false, "/some/engine"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.checker.diskCheckPath()
			if tt.wantDot {
				if got != "." {
					t.Errorf("diskCheckPath() = %q, want \".\"", got)
				}
				return
			}
			if got != tt.wantPath {
				t.Errorf("diskCheckPath() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}
