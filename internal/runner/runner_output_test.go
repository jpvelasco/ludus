package runner

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunOutput_CapturesStdout(t *testing.T) {
	r := NewRunner(false, false)
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()
	out, err := r.RunOutput(ctx, "go", "version")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "go version") {
		t.Errorf("expected output to contain 'go version', got: %s", outStr)
	}
}

func TestRunOutput_DryRun(t *testing.T) {
	r := NewRunner(false, true) // DryRun=true
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	out, err := r.RunOutput(ctx, "nonexistent-ludus-command-xyz")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}
	if string(out) != "(dry-run)" {
		t.Errorf("expected '(dry-run)', got %q", string(out))
	}
}

func TestRunQuiet_SuppressesStdout(t *testing.T) {
	r := NewRunner(false, false) // not verbose
	var stdout bytes.Buffer
	r.Stdout = &stdout
	r.Stderr = &bytes.Buffer{}
	ctx := context.Background()

	err := r.RunQuiet(ctx, "go", "version")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no stdout in quiet mode, got: %s", stdout.String())
	}
}

func TestRunQuiet_ShowsStdoutWhenVerbose(t *testing.T) {
	r := NewRunner(true, false) // verbose
	var stdout bytes.Buffer
	r.Stdout = &stdout
	r.Stderr = &bytes.Buffer{}
	ctx := context.Background()

	err := r.RunQuiet(ctx, "go", "version")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "+ go version") {
		t.Errorf("expected verbose prefix, got: %s", output)
	}
	if !strings.Contains(output, "go version go") {
		t.Errorf("expected command output in verbose mode, got: %s", output)
	}
}

func TestRunQuiet_DryRun(t *testing.T) {
	r := NewRunner(false, true) // dry-run
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	err := r.RunQuiet(ctx, "nonexistent-ludus-command-xyz")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "+ nonexistent-ludus-command-xyz") {
		t.Errorf("expected dry-run output, got: %s", stdout.String())
	}
}

func TestRunQuietWithStdin_SuppressesStdout(t *testing.T) {
	r := NewRunner(false, false) // not verbose
	var stdout bytes.Buffer
	r.Stdout = &stdout
	r.Stderr = &bytes.Buffer{}
	ctx := context.Background()

	input := strings.NewReader("hello")
	err := r.RunQuietWithStdin(ctx, input, "go", "version")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no stdout in quiet mode, got: %s", stdout.String())
	}
}

func TestRunQuietWithStdin_DryRun(t *testing.T) {
	r := NewRunner(false, true) // dry-run
	var stdout bytes.Buffer
	r.Stdout = &stdout
	ctx := context.Background()

	input := strings.NewReader("unused")
	err := r.RunQuietWithStdin(ctx, input, "nonexistent-ludus-command-xyz")
	if err != nil {
		t.Errorf("expected nil error in dry-run mode, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "+ nonexistent-ludus-command-xyz") {
		t.Errorf("expected dry-run output, got: %s", stdout.String())
	}
}
