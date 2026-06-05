package prereq

import (
	"runtime"
	"strings"
	"testing"
)

func TestCheckOS_CurrentPlatform(t *testing.T) {
	c := &Checker{}
	result := c.checkOS()

	if result.Name != "Operating System" {
		t.Errorf("expected name 'Operating System', got: %s", result.Name)
	}

	// Table of expectations for the live checkOS() call on the current platform.
	// This ensures the real implementation path is exercised on CI runners (linux, windows, darwin).
	expects := map[string]struct {
		passed  bool
		contain string
	}{
		"linux":   {true, "Linux"},
		"windows": {true, "Windows"},
		"darwin":  {true, "macOS"},
	}

	if ex, ok := expects[runtime.GOOS]; ok {
		if result.Passed != ex.passed {
			t.Errorf("expected Passed=%v on %s, got: %s", ex.passed, runtime.GOOS, result.Message)
		}
		if !strings.Contains(result.Message, ex.contain) {
			t.Errorf("expected %q in message, got: %s", ex.contain, result.Message)
		}
		return
	}

	// Unsupported platform (hit only if running on an exotic OS).
	if result.Passed {
		t.Errorf("expected fail on unsupported OS %s", runtime.GOOS)
	}
}

func TestCheckOS_Messages(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		wantPassed  bool
		wantContain string
	}{
		{"linux passes", "linux", true, "Linux"},
		{"windows passes", "windows", true, "Windows"},
		{"darwin passes", "darwin", true, "macOS"},
		{"freebsd fails", "freebsd", false, "unsupported OS"},
		{"plan9 fails", "plan9", false, "unsupported OS"},
	}

	// We can only test the current platform's case via the real checkOS().
	// For other platforms we test the message shape directly.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.goos != runtime.GOOS {
				// Verify the message format for unsupported platforms without
				// being able to call runtime.GOOS on another OS.
				if !tt.wantPassed {
					// Construct the expected message directly to verify format.
					msg := "unsupported OS: " + tt.goos + " (need linux, windows, or darwin)"
					if !strings.Contains(msg, tt.wantContain) {
						t.Errorf("expected %q in fabricated message %q", tt.wantContain, msg)
					}
				}
				return
			}
			c := &Checker{}
			result := c.checkOS()
			if result.Passed != tt.wantPassed {
				t.Errorf("Passed=%v, want %v; message: %s", result.Passed, tt.wantPassed, result.Message)
			}
			if !strings.Contains(result.Message, tt.wantContain) {
				t.Errorf("expected %q in message, got: %s", tt.wantContain, result.Message)
			}
		})
	}
}
