package prereq

import (
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

func TestCheckCrossArchEmulation_UsesConfiguredBackend(t *testing.T) {
	// When backend is explicitly "podman", cross-arch check should use podman, not docker.
	targetArch := "arm64"
	if runtime.GOARCH == "arm64" {
		targetArch = "amd64"
	}

	c := &Checker{
		Backend:    "podman",
		GameConfig: &config.GameConfig{Arch: targetArch},
	}
	result := c.checkCrossArchEmulation()

	if result.Name != "Cross-Arch Emulation" {
		t.Errorf("expected name 'Cross-Arch Emulation', got: %s", result.Name)
	}
	// Should reference podman, not docker, in the message.
	if strings.Contains(result.Message, "docker") {
		t.Errorf("expected podman-based message with podman backend, got: %s", result.Message)
	}
}

func TestCheckCrossArchEmulation_NativeArch(t *testing.T) {
	c := &Checker{
		GameConfig: &config.GameConfig{Arch: runtime.GOARCH},
	}
	result := c.checkCrossArchEmulation()
	if !result.Passed {
		t.Errorf("expected pass for native arch, got: %s", result.Message)
	}
}

func TestCheckCrossArchEmulation_NativeArchWithContainerBackend(t *testing.T) {
	// A native-arch target with a container backend must pass without requiring
	// amd64 QEMU. This guards the ordering fix: on an arm64 Linux host, a native
	// arm64 final-image build must not be blocked by the arm64→amd64 QEMU check.
	for _, backend := range []string{"docker", "podman"} {
		t.Run(backend, func(t *testing.T) {
			c := &Checker{
				Backend:    backend,
				GameConfig: &config.GameConfig{Arch: runtime.GOARCH},
			}
			result := c.checkCrossArchEmulation()
			if !result.Passed {
				t.Errorf("native %s target with %s backend should pass, got: %s", runtime.GOARCH, backend, result.Message)
			}
			if strings.Contains(result.Message, "QEMU") {
				t.Errorf("native build should not mention QEMU emulation, got: %s", result.Message)
			}
		})
	}
}

func TestCheckCrossArchEmulation_NoGameConfig(t *testing.T) {
	c := &Checker{}
	result := c.checkCrossArchEmulation()
	if !result.Passed {
		t.Errorf("expected pass with no game config, got: %s", result.Message)
	}
}

func TestCheckCrossArchEmulation_CrossArch(t *testing.T) {
	// Pick an arch that differs from the host.
	targetArch := "arm64"
	if runtime.GOARCH == "arm64" {
		targetArch = "amd64"
	}

	c := &Checker{
		GameConfig: &config.GameConfig{Arch: targetArch},
	}
	result := c.checkCrossArchEmulation()

	// On CI or machines without Docker/QEMU this may pass or fail —
	// we just verify it doesn't panic and returns a coherent result.
	if result.Name != "Cross-Arch Emulation" {
		t.Errorf("expected name 'Cross-Arch Emulation', got: %s", result.Name)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}
