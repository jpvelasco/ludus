//go:build windows

package prereq

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/dockerbuild"
)

// TestCheckWSL2_WslExeMissingWarns covers the wsl.exe-not-found branch when
// WSL2 is not the selected backend: the check degrades to a warning.
func TestCheckWSL2_WslExeMissingWarns(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	res := (&Checker{}).checkWSL2()
	if !res.Passed || !res.Warning {
		t.Fatalf("checkWSL2() = %+v, want pass+warning", res)
	}
	if !strings.Contains(res.Message, "wsl.exe not found") {
		t.Fatalf("checkWSL2() message = %q, want 'wsl.exe not found'", res.Message)
	}
}

// TestCheckWSL2_WslExeMissingRequired covers the same branch when the WSL2
// backend is selected: the check becomes a hard failure.
func TestCheckWSL2_WslExeMissingRequired(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	res := (&Checker{Backend: dockerbuild.BackendWSL2}).checkWSL2()
	if res.Passed {
		t.Fatalf("checkWSL2() = %+v, want hard failure", res)
	}
	if !strings.Contains(res.Message, "wsl.exe not found") {
		t.Fatalf("checkWSL2() message = %q, want 'wsl.exe not found'", res.Message)
	}
}
