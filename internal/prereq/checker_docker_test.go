package prereq

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
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

func TestCheckMacOSContainerBuild_NoEngineSource(t *testing.T) {
	c := &Checker{Backend: "podman"}
	result := c.checkMacOSContainerBuild()
	if result.Name != "macOS Container Build" {
		t.Errorf("expected name 'macOS Container Build', got: %s", result.Name)
	}
	if !result.Passed {
		t.Errorf("expected pass (skip) with no engine source, got: %s", result.Message)
	}
	if result.Warning {
		t.Errorf("expected no warning when skipped due to no engine source")
	}
}

func TestCheckMacOSContainerBuild_NonContainerBackend(t *testing.T) {
	c := &Checker{Backend: "native", EngineSourcePath: "/some/path"}
	result := c.checkMacOSContainerBuild()
	if !result.Passed {
		t.Errorf("expected pass (skip) for native backend, got: %s", result.Message)
	}
	if result.Warning {
		t.Errorf("native backend should not warn")
	}
}

func TestCheckMacOSContainerBuild_ToolchainMissing(t *testing.T) {
	root := t.TempDir()
	c := &Checker{
		Backend:          "podman",
		EngineSourcePath: root,
		EngineVersion:    "5.7",
		GameConfig:       &config.GameConfig{Arch: "arm64"},
	}
	result := c.checkMacOSContainerBuild()
	if result.Name != "macOS Container Build" {
		t.Errorf("unexpected name: %s", result.Name)
	}
	if !result.Passed {
		t.Errorf("expected pass+warning (not failure) for missing toolchain, got: %s", result.Message)
	}
	if !result.Warning {
		t.Errorf("expected warning flag for missing toolchain")
	}
	if !strings.Contains(result.Message, "Linux toolchain") {
		t.Errorf("expected 'Linux toolchain' in message, got: %s", result.Message)
	}
}

func TestCheckMacOSContainerBuild_ToolchainPresent(t *testing.T) {
	root := t.TempDir()
	sdkDir := filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64", "v26_clang-20.1.8-rockylinux8")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Checker{
		Backend:          "podman",
		EngineSourcePath: root,
		EngineVersion:    "5.7",
		GameConfig:       &config.GameConfig{Arch: "arm64"},
	}
	result := c.checkMacOSContainerBuild()
	if !result.Passed || result.Warning {
		t.Errorf("expected clean pass when toolchain present: passed=%v warning=%v message=%s",
			result.Passed, result.Warning, result.Message)
	}
}

func TestCheckMacOSContainerBuild_DockerBackend(t *testing.T) {
	root := t.TempDir()
	c := &Checker{
		Backend:          "docker",
		EngineSourcePath: root,
		EngineVersion:    "5.7",
	}
	result := c.checkMacOSContainerBuild()
	// Docker backend with missing toolchain → warning
	if !result.Passed {
		t.Errorf("expected pass+warning for docker backend, got failure: %s", result.Message)
	}
	if !result.Warning {
		t.Errorf("expected warning for docker backend with missing toolchain")
	}
}
