package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
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
func TestLocateProjectMissing(t *testing.T) {
	tests := []struct {
		name string
		opts BuildOptions
		want string
	}{
		{name: "configured path", opts: BuildOptions{ProjectPath: filepath.Join(t.TempDir(), "Missing.uproject")}, want: "configured project path not found"},
		{name: "Lyra auto-detect path", opts: BuildOptions{EnginePath: t.TempDir(), ProjectName: "Lyra"}, want: "Lyra.uproject not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBuilder(tt.opts, runner.NewRunner(false, true)).LocateProject()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LocateProject() error = %v, want %q", err, tt.want)
			}
		})
	}
}
