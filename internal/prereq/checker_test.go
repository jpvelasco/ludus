package prereq

import (
	"runtime"
	"strings"
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

func TestValidate_AllPass(t *testing.T) {
	results := []CheckResult{
		{Name: "A", Passed: true, Message: "ok"},
		{Name: "B", Passed: true, Message: "ok"},
	}
	if err := Validate(results); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestValidate_WithFailure(t *testing.T) {
	results := []CheckResult{
		{Name: "A", Passed: true, Message: "ok"},
		{Name: "B", Passed: false, Message: "missing"},
	}
	err := Validate(results)
	if err == nil {
		t.Fatal("expected error for failed check")
	}
	if !strings.Contains(err.Error(), "1 prerequisite check(s) failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidate_WarningsPass(t *testing.T) {
	results := []CheckResult{
		{Name: "A", Passed: true, Warning: true, Message: "heads up"},
	}
	if err := Validate(results); err != nil {
		t.Errorf("warnings should not cause failure, got: %v", err)
	}
}

func TestValidate_Empty(t *testing.T) {
	if err := Validate(nil); err != nil {
		t.Errorf("empty results should pass, got: %v", err)
	}
}

func TestCheckEngineReady(t *testing.T) {
	c := &Checker{EngineSourcePath: "/nonexistent"}
	results := c.CheckEngineReady()
	if len(results) != 1 {
		t.Fatalf("expected 1 check, got %d", len(results))
	}
	if results[0].Name == "" {
		t.Error("expected non-empty check name")
	}
}

func TestCheckGameReady(t *testing.T) {
	c := &Checker{
		EngineSourcePath: "/nonexistent",
		GameConfig:       &config.GameConfig{},
	}
	results := c.CheckGameReady()
	if len(results) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(results))
	}
}

func TestCheckDockerReady(t *testing.T) {
	c := &Checker{GameConfig: &config.GameConfig{}}
	results := c.CheckDockerReady()
	if len(results) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(results))
	}
}

func TestCheckPushReady(t *testing.T) {
	c := &Checker{GameConfig: &config.GameConfig{}}
	results := c.CheckPushReady()
	if len(results) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(results))
	}
}

func TestCheckAWSReady(t *testing.T) {
	c := &Checker{GameConfig: &config.GameConfig{}}
	results := c.CheckAWSReady()
	if len(results) != 1 {
		t.Fatalf("expected 1 check, got %d", len(results))
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
