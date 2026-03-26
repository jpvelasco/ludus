package prereq

import (
	"runtime"
	"testing"

	"github.com/devrecon/ludus/internal/config"
)

func TestCheckCrossArchEmulation_NativeArch(t *testing.T) {
	c := &Checker{
		GameConfig: &config.GameConfig{Arch: runtime.GOARCH},
	}
	result := c.checkCrossArchEmulation()
	if !result.Passed {
		t.Errorf("expected pass for native arch, got: %s", result.Message)
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
