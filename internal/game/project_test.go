package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
)

func TestLocateProject(t *testing.T) {
	t.Run("configured project path", func(t *testing.T) {
		projectPath := filepath.Join(t.TempDir(), "MyGame.uproject")
		writeTestFile(t, projectPath, "")
		b := NewBuilder(BuildOptions{ProjectPath: projectPath}, runner.NewRunner(false, true))

		got, err := b.LocateProject()
		if err != nil {
			t.Fatalf("LocateProject() error: %v", err)
		}
		if got != projectPath {
			t.Errorf("LocateProject() = %q, want %q", got, projectPath)
		}
	})

	t.Run("lyra auto-detect", func(t *testing.T) {
		enginePath := t.TempDir()
		projectPath := filepath.Join(enginePath, "Samples", "Games", "Lyra", "Lyra.uproject")
		writeTestFile(t, projectPath, "")
		b := NewBuilder(BuildOptions{EnginePath: enginePath, ProjectName: "Lyra"}, runner.NewRunner(false, true))

		got, err := b.LocateProject()
		if err != nil {
			t.Fatalf("LocateProject() error: %v", err)
		}
		if got != projectPath {
			t.Errorf("LocateProject() = %q, want %q", got, projectPath)
		}
	})

	t.Run("custom project requires explicit path", func(t *testing.T) {
		b := NewBuilder(BuildOptions{ProjectName: "MyGame"}, runner.NewRunner(false, true))

		_, err := b.LocateProject()
		if err == nil {
			t.Fatal("LocateProject() should error")
		}
		if !strings.Contains(err.Error(), "game.projectPath must be set") {
			t.Errorf("LocateProject() error = %v", err)
		}
	})
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
