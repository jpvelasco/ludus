package globals

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

// resetRunnerState clears package-level logging state between tests.
func resetRunnerState(t *testing.T) {
	t.Helper()
	CloseBuildLog()
	Cfg = nil
	NoLogs = false
	CommandName = ""
	resetBuildLogOnce()
}

// failTool installs a stub command on PATH that writes msg to stderr and
// exits with the given code.
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

func TestNewRunner_CreatesLogWhenEnabled(t *testing.T) {
	resetRunnerState(t)
	defer resetRunnerState(t)

	dir := t.TempDir()
	t.Chdir(dir)
	Cfg = config.Defaults() // logs enabled, dir ".ludus/logs"
	CommandName = "engine"

	r := NewRunner()
	if r == nil {
		t.Fatal("NewRunner returned nil")
	}
	CloseBuildLog()

	logsDir := filepath.Join(dir, ".ludus", "logs")
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		t.Fatalf("expected logs dir created: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(entries))
	}
	if filepath.Ext(entries[0].Name()) != ".log" {
		t.Errorf("expected a .log file, got %q", entries[0].Name())
	}
}

func TestNewRunner_NoLogWhenDisabled(t *testing.T) {
	resetRunnerState(t)
	defer resetRunnerState(t)

	dir := t.TempDir()
	t.Chdir(dir)
	Cfg = config.Defaults()
	disabled := false
	Cfg.Observability.Logs.Enabled = &disabled
	CommandName = "engine"

	_ = NewRunner()
	CloseBuildLog()

	if _, err := os.Stat(filepath.Join(dir, ".ludus", "logs")); !os.IsNotExist(err) {
		t.Error("expected no logs dir when logging disabled")
	}
}

func TestNewRunner_NoLogWhenNoLogsFlag(t *testing.T) {
	resetRunnerState(t)
	defer resetRunnerState(t)

	dir := t.TempDir()
	t.Chdir(dir)
	Cfg = config.Defaults()
	NoLogs = true
	CommandName = "engine"

	_ = NewRunner()
	CloseBuildLog()

	if _, err := os.Stat(filepath.Join(dir, ".ludus", "logs")); !os.IsNotExist(err) {
		t.Error("expected no logs dir when --no-logs set")
	}
}

func TestNewRunner_SingleLogAcrossCalls(t *testing.T) {
	resetRunnerState(t)
	defer resetRunnerState(t)

	dir := t.TempDir()
	t.Chdir(dir)
	Cfg = config.Defaults()
	CommandName = "run"

	_ = NewRunner()
	_ = NewRunner()
	_ = NewRunner()
	CloseBuildLog()

	entries, err := os.ReadDir(filepath.Join(dir, ".ludus", "logs"))
	if err != nil {
		t.Fatalf("expected logs dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected a single shared log file across NewRunner calls, got %d", len(entries))
	}
}
func TestLogsDir(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{name: "nil config", want: ".ludus/logs"},
		{name: "default config", cfg: config.Defaults(), want: ".ludus/logs"},
		{name: "configured directory", cfg: configWithLogsDir("artifacts/logs"), want: "artifacts/logs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := Cfg
			t.Cleanup(func() { Cfg = previous })
			Cfg = tt.cfg
			if got := LogsDir(); got != tt.want {
				t.Errorf("LogsDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func configWithLogsDir(dir string) *config.Config {
	cfg := config.Defaults()
	cfg.Observability.Logs.Dir = dir
	return cfg
}

func TestSectionLogWithoutActiveLog(t *testing.T) {
	resetRunnerState(t)
	defer resetRunnerState(t)

	SectionLog("build")
}

func TestNewRunner_UsesDefaultLogDirAndName(t *testing.T) {
	resetRunnerState(t)
	defer resetRunnerState(t)

	dir := t.TempDir()
	t.Chdir(dir)
	Cfg = &config.Config{} // logs enabled by default; Dir and CommandName empty

	_ = NewRunner()
	CloseBuildLog()

	entries, err := os.ReadDir(filepath.Join(dir, ".ludus", "logs"))
	if err != nil {
		t.Fatalf("expected default logs dir created: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(entries))
	}
	if !strings.HasPrefix(entries[0].Name(), "ludus-") {
		t.Errorf("log file name = %q, want ludus- prefix", entries[0].Name())
	}
}

func TestNewRunner_LogOpenFailureIsNonFatal(t *testing.T) {
	resetRunnerState(t)
	defer resetRunnerState(t)

	dir := t.TempDir()
	t.Chdir(dir)
	blocker := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("creating blocker file: %v", err)
	}
	Cfg = &config.Config{}
	Cfg.Observability.Logs.Dir = filepath.Join(blocker, "logs")

	r := NewRunner()
	if r == nil {
		t.Fatal("NewRunner() = nil, want a runner even when the log cannot open")
	}
	CloseBuildLog()
}

func TestSectionLogWithActiveLog(t *testing.T) {
	resetRunnerState(t)
	defer resetRunnerState(t)

	dir := t.TempDir()
	t.Chdir(dir)
	Cfg = config.Defaults()
	CommandName = "engine"

	_ = NewRunner()
	SectionLog("stage")
	CloseBuildLog()
}

// captureStderr redirects os.Stderr into a temp file for the rest of the test
// and returns the file path, for asserting on what the terminal would see.
func captureStderr(t *testing.T) string {
	t.Helper()
	origStderr := os.Stderr
	errTmp, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = errTmp
	t.Cleanup(func() { _ = errTmp.Close(); os.Stderr = origStderr })
	return errTmp.Name()
}

// maskingRunnerState configures the globals for human-mode stderr masking.
func maskingRunnerState(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	Cfg = config.Defaults()
	Cfg.Privacy.MaskAccountID = true
	JSONOutput = false
	CommandName = "engine"
}

// TestNewRunnerMasksChildStderr pins the #554 contract: with human-mode
// masking enabled, child-process stderr (e.g. docker push progress naming the
// ECR repository) is masked before it reaches the terminal.
func TestNewRunnerMasksChildStderr(t *testing.T) {
	resetRunnerState(t)
	defer resetRunnerState(t)

	maskingRunnerState(t)
	stderrPath := captureStderr(t)

	failTool(t, "123456789012.dkr.ecr.us-east-1.amazonaws.com/ludus-server", 1)

	r := NewRunner()
	_ = r.RunQuietErr(testContext(t), "ludus-fail-tool")

	got, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("no child stderr captured")
	}
	if strings.Contains(string(got), "123456789012") {
		t.Errorf("child stderr not masked: %q", got)
	}
	if !strings.Contains(string(got), "************.dkr.ecr.us-east-1.amazonaws.com") {
		t.Errorf("expected masked ECR host in child stderr, got: %q", got)
	}
}

// testContext returns a context cancelled when the test ends.
func testContext(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}

// TestNewRunnerFlushesUnterminatedStderrOnCommandEnd pins the #711 contract:
// with masking active, a child's final unterminated stderr line (written by
// the child straight into the runner's stderr writer) must reach the terminal
// and log sink once the command completes. Without an end-of-command flush
// that message was buffered forever and lost.
func TestNewRunnerFlushesUnterminatedStderrOnCommandEnd(t *testing.T) {
	resetRunnerState(t)
	defer resetRunnerState(t)

	maskingRunnerState(t)
	stderrPath := captureStderr(t)

	r := NewRunner()
	// A child wrote an unterminated final error line to stderr and exited:
	// no newline, so the masking writer buffers it.
	msg := "tail: 123456789012.dkr.ecr.us-east-1.amazonaws.com/server"
	if _, err := io.WriteString(r.Stderr, msg); err != nil {
		t.Fatal(err)
	}

	// While no command is in flight the incomplete line must stay buffered —
	// the masking guarantee does not allow early unmasked leaks.
	assertFileEmpty(t, stderrPath)

	// Any subsequent command completing through this runner flushes the tail.
	if err := r.RunQuietErr(testContext(t), "go", "version"); err != nil {
		t.Fatalf("running child: %v", err)
	}

	got, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "tail: ************.dkr.ecr.us-east-1.amazonaws.com/server") {
		t.Errorf("unterminated stderr line not flushed and masked at command end, got: %q", got)
	}
	if strings.Contains(string(got), "123456789012") {
		t.Errorf("flushed stderr leaked the account ID: %q", got)
	}
}

// assertFileEmpty fails the test when path holds any bytes.
func assertFileEmpty(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("incomplete stderr line forwarded before a command completed: %q", data)
	}
}
