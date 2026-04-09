package prereq

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/config"
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
	// When backend is podman but podman is not in PATH, it should fail.
	c := &Checker{Backend: "podman"}
	// Temporarily clear PATH to simulate podman not found.
	t.Setenv("PATH", "")
	result := c.checkPodman()
	if result.Passed {
		// On some systems podman might still be found via other mechanisms.
		// Skip if podman is actually available.
		t.Skip("podman found despite empty PATH")
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

func TestIsLyraProject(t *testing.T) {
	t.Run("has Lyra.uproject", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "Lyra.uproject"), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
		if !isLyraProject(dir) {
			t.Error("expected true for dir with Lyra.uproject")
		}
	})

	t.Run("has DefaultGameData", func(t *testing.T) {
		dir := t.TempDir()
		contentDir := filepath.Join(dir, "Content")
		if err := os.MkdirAll(contentDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(contentDir, "DefaultGameData.uasset"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if !isLyraProject(dir) {
			t.Error("expected true for dir with DefaultGameData.uasset")
		}
	})

	t.Run("empty dir", func(t *testing.T) {
		if isLyraProject(t.TempDir()) {
			t.Error("expected false for empty dir")
		}
	})

	t.Run("nonexistent dir", func(t *testing.T) {
		if isLyraProject(filepath.Join(t.TempDir(), "nope")) {
			t.Error("expected false for nonexistent dir")
		}
	})
}

func TestResolveContentDir(t *testing.T) {
	t.Run("with project path", func(t *testing.T) {
		c := &Checker{
			GameConfig: &config.GameConfig{
				ProjectPath: filepath.Join("projects", "MyGame", "MyGame.uproject"),
			},
		}
		got := c.resolveContentDir()
		want := filepath.Join("projects", "MyGame", "Content")
		if got != want {
			t.Errorf("resolveContentDir() = %q, want %q", got, want)
		}
	})

	t.Run("Lyra with engine source", func(t *testing.T) {
		c := &Checker{
			EngineSourcePath: "/ue5",
			GameConfig:       &config.GameConfig{ProjectName: "Lyra"},
		}
		got := c.resolveContentDir()
		want := filepath.Join("/ue5", "Samples", "Games", "Lyra", "Content")
		if got != want {
			t.Errorf("resolveContentDir() = %q, want %q", got, want)
		}
	})

	t.Run("default Lyra with engine source", func(t *testing.T) {
		c := &Checker{
			EngineSourcePath: "/ue5",
			GameConfig:       &config.GameConfig{},
		}
		got := c.resolveContentDir()
		want := filepath.Join("/ue5", "Samples", "Games", "Lyra", "Content")
		if got != want {
			t.Errorf("resolveContentDir() = %q, want %q", got, want)
		}
	})

	t.Run("non-Lyra without project path", func(t *testing.T) {
		c := &Checker{
			GameConfig: &config.GameConfig{ProjectName: "CustomGame"},
		}
		got := c.resolveContentDir()
		if got != "" {
			t.Errorf("resolveContentDir() = %q, want empty", got)
		}
	})

	t.Run("nil game config", func(t *testing.T) {
		c := &Checker{EngineSourcePath: "/ue5"}
		got := c.resolveContentDir()
		want := filepath.Join("/ue5", "Samples", "Games", "Lyra", "Content")
		if got != want {
			t.Errorf("resolveContentDir() = %q, want %q", got, want)
		}
	})
}
