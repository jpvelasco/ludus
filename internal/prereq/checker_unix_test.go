//go:build !windows

package prereq

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/dockerbuild"
	"github.com/jpvelasco/ludus/internal/toolchain"
)

// TestCheckWSL2_NonWindowsWarns covers the early return for non-Windows hosts:
// WSL2 is only meaningful on Windows, so the check passes with a warning.
func TestCheckWSL2_NonWindowsWarns(t *testing.T) {
	res := (&Checker{}).checkWSL2()
	if !res.Passed || !res.Warning {
		t.Fatalf("checkWSL2() = %+v, want pass+warning", res)
	}
	if !strings.Contains(res.Message, "only available on Windows") {
		t.Fatalf("checkWSL2() message = %q, want 'only available on Windows'", res.Message)
	}
}

// TestCheckDocker_NotFoundWithOtherBackendWarns covers the non-Windows branch
// where docker is missing but a different backend is selected: warning, not
// failure.
func TestCheckDocker_NotFoundWithOtherBackendWarns(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	res := (&Checker{Backend: dockerbuild.BackendPodman}).checkDocker()
	if !res.Passed || !res.Warning {
		t.Fatalf("checkDocker() = %+v, want pass+warning", res)
	}
	if !strings.Contains(res.Message, "not needed for podman backend") {
		t.Fatalf("checkDocker() message = %q, want 'not needed for podman backend'", res.Message)
	}
}

// TestCheckDocker_NotFoundHardFails covers the non-Windows branch where docker
// is missing and no other backend is selected: hard failure.
func TestCheckDocker_NotFoundHardFails(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	res := (&Checker{}).checkDocker()
	if res.Passed {
		t.Fatalf("checkDocker() = %+v, want hard failure", res)
	}
	if !strings.Contains(res.Message, "docker not found in PATH") {
		t.Fatalf("checkDocker() message = %q, want 'docker not found in PATH'", res.Message)
	}
}

// TestCheckOS_HostDetected covers the host's GOOS branch of checkOS on
// non-Windows CI legs (Linux on ubuntu, darwin on macOS).
func TestCheckOS_HostDetected(t *testing.T) {
	res := (&Checker{}).checkOS()
	if !res.Passed || res.Warning {
		t.Fatalf("checkOS() = %+v, want clean pass", res)
	}
	if !strings.Contains(res.Message, "detected") {
		t.Fatalf("checkOS() message = %q, want host 'detected'", res.Message)
	}
}

// TestToolchainNotFoundResult_NonWindowsFixOff covers the non-Windows result
// path without --fix: the guidance suffix is appended and the check fails.
func TestToolchainNotFoundResult_NonWindowsFixOff(t *testing.T) {
	tc := toolchain.CheckResult{Message: "toolchain v26 not found"}
	res := (&Checker{}).toolchainNotFoundResult(tc)
	if res.Passed || res.Warning {
		t.Fatalf("toolchainNotFoundResult() = %+v, want hard failure", res)
	}
	if !strings.Contains(res.Message, "run with --fix for instructions") {
		t.Fatalf("toolchainNotFoundResult() message = %q, want '--fix' guidance", res.Message)
	}
}

// TestToolchainNotFoundResult_NonWindowsFixOn covers the non-Windows path with
// --fix set: the plain message is returned without the guidance suffix.
func TestToolchainNotFoundResult_NonWindowsFixOn(t *testing.T) {
	tc := toolchain.CheckResult{Message: "toolchain v26 not found"}
	res := (&Checker{Fix: true}).toolchainNotFoundResult(tc)
	if res.Passed || res.Warning {
		t.Fatalf("toolchainNotFoundResult() = %+v, want hard failure", res)
	}
	if strings.Contains(res.Message, "run with --fix") {
		t.Fatalf("toolchainNotFoundResult() message = %q, want no --fix suffix with Fix set", res.Message)
	}
}
