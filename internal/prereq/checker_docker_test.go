package prereq

import (
	"strings"
	"testing"
)

func TestCheckDocker_BackendDowngradeToWarning(t *testing.T) {
	// When using podman backend, Docker daemon being down should be a warning, not failure.
	c := &Checker{Backend: "podman"}
	result := c.checkDocker()

	// We can't guarantee Docker is or isn't available in test environments,
	// but if Docker is in PATH but daemon is down, the backend field should downgrade to warning.
	if result.Name != "Docker" {
		t.Errorf("expected name 'Docker', got: %s", result.Name)
	}
	// When backend is podman, Docker checks must never be Passed=false.
	if !result.Passed {
		t.Errorf("expected Docker check to pass (as warning) with podman backend, got: %s", result.Message)
	}
}

func TestCheckDocker_NoBackend(t *testing.T) {
	// Without a backend set, Docker check may fail if daemon is down.
	c := &Checker{}
	result := c.checkDocker()
	if result.Name != "Docker" {
		t.Errorf("expected name 'Docker', got: %s", result.Name)
	}
	// On Windows, if docker isn't found, it still passes as warning.
	// If it's found but daemon is down, it fails. Either way, we just verify no panic.
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestCheckPodman_BackendPodmanNotFound(t *testing.T) {
	// When backend is podman but podman is not in PATH, it should fail —
	// unless the Windows fallback finds Podman at its default install location.
	c := &Checker{Backend: "podman"}
	t.Setenv("PATH", "")
	result := c.checkPodman()
	if result.Passed || strings.Contains(result.Message, "podman found") {
		// Podman found via fallback path or other mechanism.
		t.Skip("podman found despite empty PATH (Windows fallback or system lookup)")
	}
	if !strings.Contains(result.Message, "not found in PATH") {
		t.Errorf("expected 'not found' message, got: %s", result.Message)
	}
}

func TestCheckPodman_BackendDockerNotFound(t *testing.T) {
	// When backend is docker and podman is not in PATH, podman check should be a warning.
	c := &Checker{Backend: "docker"}
	t.Setenv("PATH", "")
	result := c.checkPodman()
	if !result.Passed {
		t.Errorf("expected pass (warning) when backend is docker and podman not found, got: %s", result.Message)
	}
	if !result.Warning {
		t.Errorf("expected warning flag set")
	}
}
