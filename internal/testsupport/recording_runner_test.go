package testsupport

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/engine"
)

func TestRecordingRunnerCapturesCommands(t *testing.T) {
	r, getLines := RecordingRunner()

	// Verify it's in dry-run mode
	if !r.DryRun {
		t.Error("RecordingRunner should be in dry-run mode")
	}

	// Run a fake engine builder to trigger command echoing
	root := FakeEngineTree(t, WithoutSetup())
	builder := engine.NewBuilder(engine.BuildOptions{
		SourcePath: root,
		MaxJobs:    1,
		Verbose:    true,
	}, r)

	// Call GenerateProjectFiles (won't execute, just record)
	_ = builder.GenerateProjectFiles(context.Background())

	// Verify commands were recorded
	lines := getLines()
	if len(lines) == 0 {
		t.Fatal("expected recorded commands, got none")
	}

	// Should have recorded GenerateProjectFiles
	hasGenerate := false
	for _, line := range lines {
		if strings.Contains(line, "GenerateProjectFiles") {
			hasGenerate = true
			break
		}
	}
	if !hasGenerate {
		t.Errorf("expected GenerateProjectFiles in lines, got: %v", lines)
	}
}

func TestRecordingRunnerDoesNotSpawnProcesses(t *testing.T) {
	r, _ := RecordingRunner()

	// Try to run a non-existent command; should not fail because it's dry-run
	err := r.Run(context.Background(), "/nonexistent/command", "arg1", "arg2")
	if err != nil {
		t.Errorf("dry-run should not execute, got error: %v", err)
	}
}

func TestRecordingRunnerParsesMultipleLines(t *testing.T) {
	r, getLines := RecordingRunner()

	// Manually write some commands via the runner's echo mechanism
	// by constructing fake commands that will be echoed
	root := FakeEngineTree(t, WithoutSetup())
	builder := engine.NewBuilder(engine.BuildOptions{
		SourcePath: root,
		MaxJobs:    1,
		Verbose:    true,
	}, r)

	_ = builder.GenerateProjectFiles(context.Background())
	_ = builder.GenerateProjectFiles(context.Background())

	lines := getLines()
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines, got %d: %v", len(lines), lines)
	}
}
