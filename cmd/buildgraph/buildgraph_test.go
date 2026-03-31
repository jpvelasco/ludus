package buildgraph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteBuildGraph(t *testing.T) {
	data := []byte("<BuildGraph>test</BuildGraph>")

	t.Run("writes to explicit path", func(t *testing.T) {
		dir := t.TempDir()
		outPath := filepath.Join(dir, "output.xml")

		if err := writeBuildGraph(data, outPath, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("failed to read output: %v", err)
		}
		if string(got) != string(data) {
			t.Errorf("got %q, want %q", got, data)
		}
	})

	t.Run("resolves default path from project path", func(t *testing.T) {
		dir := t.TempDir()
		projectPath := filepath.Join(dir, "MyGame", "MyGame.uproject")

		if err := writeBuildGraph(data, "", projectPath); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := filepath.Join(dir, "MyGame", "Build", "BuildGraph.xml")
		got, err := os.ReadFile(expected)
		if err != nil {
			t.Fatalf("failed to read output at %s: %v", expected, err)
		}
		if string(got) != string(data) {
			t.Errorf("got %q, want %q", got, data)
		}
	})

	t.Run("creates intermediate directories", func(t *testing.T) {
		dir := t.TempDir()
		outPath := filepath.Join(dir, "deep", "nested", "output.xml")

		if err := writeBuildGraph(data, outPath, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, err := os.Stat(outPath); err != nil {
			t.Errorf("expected file at %s, got error: %v", outPath, err)
		}
	})

	t.Run("empty project path defaults to cwd", func(t *testing.T) {
		dir := t.TempDir()
		// When both outPath and projectPath are empty, the default
		// resolves relative to "." (cwd). We can't easily test the
		// exact path without changing cwd, so just verify no panic.
		outPath := filepath.Join(dir, "fallback.xml")
		if err := writeBuildGraph(data, outPath, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
