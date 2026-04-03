package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
)

func TestApplyDDCConfig_ModeNone(t *testing.T) {
	b := NewBuilder(BuildOptions{DDCMode: "none", DDCPath: "/some/path"}, &runner.Runner{})
	restore, err := b.applyDDCConfig("/fake/project.uproject")
	if err != nil {
		t.Fatalf("applyDDCConfig() error: %v", err)
	}
	restore()
}

func TestApplyDDCConfig_EmptyPath(t *testing.T) {
	b := NewBuilder(BuildOptions{DDCMode: "local", DDCPath: ""}, &runner.Runner{})
	restore, err := b.applyDDCConfig("/fake/project.uproject")
	if err != nil {
		t.Fatalf("applyDDCConfig() error: %v", err)
	}
	restore()
}

func TestApplyDDCConfig_LocalWithPath(t *testing.T) {
	projectDir := t.TempDir()
	configDir := filepath.Join(projectDir, "Config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	iniPath := filepath.Join(configDir, "DefaultEngine.ini")
	original := "[/Script/Engine.GameEngine]\nSomeSetting=true\n"
	if err := os.WriteFile(iniPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	ddcDir := filepath.Join(t.TempDir(), "ddc")
	projectPath := filepath.Join(projectDir, "MyGame.uproject")

	b := NewBuilder(BuildOptions{DDCMode: "local", DDCPath: ddcDir}, &runner.Runner{})
	restore, err := b.applyDDCConfig(projectPath)
	if err != nil {
		t.Fatalf("applyDDCConfig() error: %v", err)
	}

	// DDC directory should be created
	if _, err := os.Stat(ddcDir); err != nil {
		t.Errorf("DDC directory not created: %v", err)
	}

	// ini should be patched
	data, err := os.ReadFile(iniPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "DerivedDataBackendGraph") {
		t.Error("ini should contain DDC section after applyDDCConfig")
	}

	// restore should revert
	restore()
	data, err = os.ReadFile(iniPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("restore did not revert ini: got %q, want %q", string(data), original)
	}
}

func TestApplyDDCConfig_MissingIni(t *testing.T) {
	projectDir := t.TempDir()
	// No Config directory or ini file
	ddcDir := filepath.Join(t.TempDir(), "ddc")
	projectPath := filepath.Join(projectDir, "MyGame.uproject")

	b := NewBuilder(BuildOptions{DDCMode: "local", DDCPath: ddcDir}, &runner.Runner{})
	restore, err := b.applyDDCConfig(projectPath)
	if err != nil {
		t.Fatalf("applyDDCConfig() should not error for missing ini: %v", err)
	}
	restore() // should be a no-op
}

func TestApplyDDCConfig_EmptyMode(t *testing.T) {
	b := NewBuilder(BuildOptions{DDCMode: "", DDCPath: "/some/path"}, &runner.Runner{})
	restore, err := b.applyDDCConfig("/fake/project.uproject")
	if err != nil {
		t.Fatalf("applyDDCConfig() error: %v", err)
	}
	restore()
}
