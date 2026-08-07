package buildgraph

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestWriteBuildGraph_MkdirAllFails(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	err := writeBuildGraph([]byte("<graph/>"), filepath.Join(blocker, "out.xml"), "")
	if err == nil {
		t.Fatal("expected error when output dir path is a file")
	}
	if !strings.Contains(err.Error(), "creating output directory") {
		t.Errorf("error = %v, want it to describe the directory failure", err)
	}
}

func TestWriteBuildGraph_WriteFileFails(t *testing.T) {
	// Put a directory at the target path; writing to a directory path fails.
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.xml")
	if err := os.Mkdir(outPath, 0755); err != nil {
		t.Fatal(err)
	}

	err := writeBuildGraph([]byte("<graph/>"), outPath, "")
	if err == nil {
		t.Fatal("expected error when output path is a directory")
	}
	if !strings.Contains(err.Error(), "writing") {
		t.Errorf("error = %v, want it to describe the write failure", err)
	}
}

func TestRunBuildGraph_ToStdout(t *testing.T) {
	cfg := config.Defaults()
	cfg.Engine.SourcePath = "/opt/unreal-engine"
	cfg.Game.ProjectPath = "/opt/unreal-engine/Samples/Games/Lyra/Lyra.uproject"
	globals.SetGlobals(t, cfg)
	toStdout = true
	t.Cleanup(func() { toStdout = false })

	output, err := captureStdout(t, func() error { return runBuildGraph(nil, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Engine") {
		t.Errorf("stdout missing Engine agent, got: %q", output)
	}
}

func TestRunBuildGraph_WritesFile(t *testing.T) {
	cfg := config.Defaults()
	cfg.Engine.SourcePath = "/opt/unreal-engine"
	cfg.Game.ProjectPath = filepath.Join(t.TempDir(), "MyGame", "MyGame.uproject")
	globals.SetGlobals(t, cfg)
	toStdout = false

	if err := runBuildGraph(nil, nil); err != nil {
		t.Fatalf("runBuildGraph() error = %v", err)
	}
	// Default output resolves under the project dir.
	got, err := os.ReadFile(filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "Build", "BuildGraph.xml"))
	if err != nil {
		t.Fatalf("expected BuildGraph.xml to be written: %v", err)
	}
	if !strings.Contains(string(got), "Engine") {
		t.Errorf("file missing Engine agent, got: %q", got)
	}
}

func TestRunBuildGraph_ValidationError(t *testing.T) {
	globals.SetGlobals(t, config.Defaults())

	if err := runBuildGraph(nil, nil); err == nil {
		t.Fatal("expected validation error when engine source path is empty")
	}
}

func captureStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	runErr := run()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previous
	t.Cleanup(func() { os.Stdout = previous })
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}
