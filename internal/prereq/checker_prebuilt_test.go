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

func TestPrebuiltImageGuardedChecksPass(t *testing.T) {
	// Reproduces #394 at the unit level: with a prebuilt engine image and no host
	// engine source, the three checks that previously demanded it must pass. We
	// call the guarded checks directly rather than RunAll() to avoid RunAll's
	// AWS/Docker subprocess calls (E2E territory, and they hang in CI).
	c := NewChecker("", "", false, nil)
	c.PrebuiltImage = true

	checks := map[string]CheckResult{
		"Engine Source": c.checkEngineSource(),
		"Toolchain":     c.checkToolchain(),
		"Disk Space (path)": func() CheckResult {
			// diskCheckPath must not point at a missing engine source.
			if c.diskCheckPath() != "." {
				return CheckResult{Passed: false, Message: "diskCheckPath did not fall back to \".\""}
			}
			return CheckResult{Passed: true}
		}(),
	}
	for name, res := range checks {
		if !res.Passed {
			t.Errorf("%s should pass with prebuilt image: %s", name, res.Message)
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
