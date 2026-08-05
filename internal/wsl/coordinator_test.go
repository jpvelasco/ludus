package wsl

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// newTestRunner returns a runner with output silenced so tests stay clean.
// Dry-run mode makes every command a no-op; real mode executes the fake
// wsl.exe stub placed on PATH by testsupport.FakeTool.
func newTestRunner(t *testing.T, dryRun bool) *runner.Runner {
	t.Helper()
	r := runner.NewRunner(false, dryRun)
	r.Stdout = io.Discard
	r.Stderr = io.Discard
	return r
}

func TestNewSelectsDefaultDistro(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{
		Stdout: "* Ubuntu          Running         2",
	})

	w, err := New(newTestRunner(t, true), "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if w.Distro != "Ubuntu" {
		t.Errorf("Distro = %q, want Ubuntu", w.Distro)
	}
	if w.Runner == nil {
		t.Error("New() Runner is nil")
	}
}

func TestNewOverrideNotFound(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{
		Stdout: "* Ubuntu          Running         2",
	})

	_, err := New(newTestRunner(t, true), "Debian")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("New() error = %v, want 'not found'", err)
	}
}

func TestNewUnavailableWhenCommandFails(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})

	_, err := New(newTestRunner(t, true), "")
	if err == nil || !strings.Contains(err.Error(), "WSL2 is not available") {
		t.Errorf("New() error = %v, want 'WSL2 is not available'", err)
	}
}

func TestNewUnavailableWhenNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := New(newTestRunner(t, true), "")
	if err == nil || !strings.Contains(err.Error(), "WSL2 is not available") {
		t.Errorf("New() error = %v, want 'WSL2 is not available'", err)
	}
}

func TestWSL2RunEcho(t *testing.T) {
	r, lines := testsupport.RecordingRunner()
	w := &WSL2{Distro: "Ubuntu", Runner: r}

	if err := w.Run(context.Background(), "ls", "-la"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := strings.Join(lines(), " ")
	if !strings.Contains(got, "wsl.exe -d Ubuntu -e ls -la") {
		t.Errorf("Run() echoed %q, want wsl.exe -d Ubuntu -e ls -la", got)
	}
}

func TestWSL2RunBashEcho(t *testing.T) {
	r, lines := testsupport.RecordingRunner()
	w := &WSL2{Distro: "Debian", Runner: r}

	if err := w.RunBash(context.Background(), "echo hello"); err != nil {
		t.Fatalf("RunBash() error = %v", err)
	}
	got := strings.Join(lines(), " ")
	if !strings.Contains(got, "wsl.exe -d Debian bash -c echo hello") {
		t.Errorf("RunBash() echoed %q, want wsl.exe -d Debian bash -c echo hello", got)
	}
}

func TestWSL2RunOutputEcho(t *testing.T) {
	r, lines := testsupport.RecordingRunner()
	w := &WSL2{Distro: "Ubuntu", Runner: r}

	out, err := w.RunOutput(context.Background(), "df", "-BG")
	if err != nil {
		t.Fatalf("RunOutput() error = %v", err)
	}
	if len(out) == 0 {
		t.Error("RunOutput() returned empty output")
	}
	got := strings.Join(lines(), " ")
	if !strings.Contains(got, "wsl.exe -d Ubuntu -e df -BG") {
		t.Errorf("RunOutput() echoed %q, want wsl.exe -d Ubuntu -e df -BG", got)
	}
}

func TestEnsureDepsPresent(t *testing.T) {
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, true)}

	if err := w.EnsureDeps(context.Background()); err != nil {
		t.Errorf("EnsureDeps() error = %v, want nil", err)
	}
}

func TestEnsureDepsInstallFailure(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, false)}

	if err := w.EnsureDeps(context.Background()); err == nil {
		t.Error("EnsureDeps() error = nil, want install failure")
	}
}

func TestEnsureRuntimeDepsPresent(t *testing.T) {
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, true)}

	if err := w.EnsureRuntimeDeps(context.Background()); err != nil {
		t.Errorf("EnsureRuntimeDeps() error = %v, want nil", err)
	}
}

func TestEnsureRuntimeDepsInstallFailure(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, false)}

	if err := w.EnsureRuntimeDeps(context.Background()); err == nil {
		t.Error("EnsureRuntimeDeps() error = nil, want install failure")
	}
}

func TestDiskFreeGB(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "  250G"})
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, false)}

	got, err := w.DiskFreeGB(context.Background())
	if err != nil {
		t.Fatalf("DiskFreeGB() error = %v", err)
	}
	if got != 250 {
		t.Errorf("DiskFreeGB() = %v, want 250", got)
	}
}

func TestHasRsync(t *testing.T) {
	tests := []struct {
		name     string
		behavior testsupport.ToolBehavior
		want     bool
	}{
		{"present", testsupport.ToolBehavior{}, true},
		{"missing", testsupport.ToolBehavior{ExitCode: 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTool(t, "wsl.exe", tt.behavior)
			w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, false)}

			if got := w.HasRsync(context.Background()); got != tt.want {
				t.Errorf("HasRsync() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpandHomePathsReplacesHome(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "/home/user"})
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, false)}

	got, err := w.ExpandHomePaths(context.Background(), "$HOME/ludus/engine", "/plain/path")
	if err != nil {
		t.Fatalf("ExpandHomePaths() error = %v", err)
	}
	if got[0] != "/home/user/ludus/engine" {
		t.Errorf("ExpandHomePaths()[0] = %q, want /home/user/ludus/engine", got[0])
	}
	if got[1] != "/plain/path" {
		t.Errorf("ExpandHomePaths()[1] = %q, want /plain/path", got[1])
	}
}

func TestExpandHomePathsResolveError(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, false)}

	if _, err := w.ExpandHomePaths(context.Background(), "$HOME/ludus"); err == nil {
		t.Error("ExpandHomePaths() error = nil, want resolve error")
	}
}

func TestExpandHomePathsEmptyHome(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: ""})
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, false)}

	_, err := w.ExpandHomePaths(context.Background(), "$HOME/ludus")
	if err == nil || !strings.Contains(err.Error(), "empty string") {
		t.Errorf("ExpandHomePaths() error = %v, want 'resolved to empty string'", err)
	}
}
