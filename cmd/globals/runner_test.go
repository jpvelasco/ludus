package globals

import (
	"os"
	"path/filepath"
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
