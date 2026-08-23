package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

// failTool installs a stub command on PATH that writes msg to stderr and
// exits with the given code. (testsupport can't be imported here: it depends
// on this package.)
func failTool(t *testing.T, msg string, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	var name, script string
	if runtime.GOOS == "windows" {
		name = "ludus-fail-tool.bat"
		script = fmt.Sprintf("@echo off\r\necho %s 1>&2\r\nexit /b %d\r\n", msg, exitCode)
	} else {
		name = "ludus-fail-tool"
		script = fmt.Sprintf("#!/bin/sh\necho %s >&2\nexit %d\n", msg, exitCode)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// TestRunQuietErr_WrapsStderr pins the error-classification contract: a
// failing command's stderr must be part of the returned error so callers can
// match AWS CLI exception names.
func TestRunQuietErr_WrapsStderr(t *testing.T) {
	failTool(t, "RepositoryAlreadyExistsException", 255)

	r := NewRunner(false, false)
	r.Stdout = io.Discard
	r.Stderr = io.Discard

	err := r.RunQuietErr(context.Background(), "ludus-fail-tool")
	if err == nil {
		t.Fatal("RunQuietErr() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "RepositoryAlreadyExistsException") {
		t.Errorf("RunQuietErr() error = %v, want stderr wrapped into the error", err)
	}
}

// TestRunQuietErr_TeesStderr ensures live visibility is preserved while the
// stderr is also captured.
func TestRunQuietErr_TeesStderr(t *testing.T) {
	failTool(t, "boom", 1)

	var stderr bytes.Buffer
	r := NewRunner(false, false)
	r.Stdout = io.Discard
	r.Stderr = &stderr

	_ = r.RunQuietErr(context.Background(), "ludus-fail-tool")
	if !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr not tee'd to runner writer, got: %q", stderr.String())
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
