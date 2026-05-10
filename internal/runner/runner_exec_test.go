package runner

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRun_Success(t *testing.T) {
	r := NewRunner(false, false)
	ctx := context.Background()
	err := r.Run(ctx, "go", "version")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestRun_Failure(t *testing.T) {
	r := NewRunner(false, false)
	ctx := context.Background()
	err := r.Run(ctx, "nonexistent-ludus-command-xyz")
	if err == nil {
		t.Error("expected non-nil error, got nil")
	}
}

func TestDryRun_DoesNotExecute(t *testing.T) {
	r := NewRunner(false, true) // DryRun=true
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	// Use a command that would fail if executed
	err := r.Run(ctx, "nonexistent-ludus-command-xyz")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}

	// Verify the command was printed
	output := stdout.String()
	if !strings.Contains(output, "+ nonexistent-ludus-command-xyz") {
		t.Errorf("expected stdout to contain '+ nonexistent-ludus-command-xyz', got: %s", output)
	}
}

func TestVerbose_PrintsCommand(t *testing.T) {
	r := NewRunner(true, false) // Verbose=true
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	err := r.Run(ctx, "go", "version")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	// Verify the command was printed
	output := stdout.String()
	if !strings.Contains(output, "+ go version") {
		t.Errorf("expected stdout to contain '+ go version', got: %s", output)
	}
}

func TestRunInDir_Success(t *testing.T) {
	r := NewRunner(false, false)
	var stdout bytes.Buffer
	r.Stdout = &stdout
	r.Stderr = &bytes.Buffer{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	err := r.RunInDir(ctx, tmpDir, "go", "version")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestRunInDir_DryRun(t *testing.T) {
	r := NewRunner(false, true) // DryRun=true
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	tmpDir := t.TempDir()
	err := r.RunInDir(ctx, tmpDir, "nonexistent-ludus-command-xyz", "arg1")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "+ cd") {
		t.Errorf("expected dry-run output to contain '+ cd', got: %s", output)
	}
	if !strings.Contains(output, "nonexistent-ludus-command-xyz") {
		t.Errorf("expected dry-run output to contain command name, got: %s", output)
	}
}

func TestRunWithStdin_Success(t *testing.T) {
	r := NewRunner(false, false)
	var stdout bytes.Buffer
	r.Stdout = &stdout
	r.Stderr = &bytes.Buffer{}
	ctx := context.Background()

	input := strings.NewReader("hello from stdin")
	err := r.RunWithStdin(ctx, input, "go", "version")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestRunWithStdin_DryRun(t *testing.T) {
	r := NewRunner(false, true) // DryRun=true
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	input := strings.NewReader("unused input")
	err := r.RunWithStdin(ctx, input, "nonexistent-ludus-command-xyz")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "+ nonexistent-ludus-command-xyz") {
		t.Errorf("expected dry-run output to contain command, got: %s", output)
	}
}
