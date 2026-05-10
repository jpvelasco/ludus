package prereq

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devrecon/ludus/internal/config"
)

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
