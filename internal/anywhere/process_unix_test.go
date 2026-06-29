//go:build !windows

package anywhere

import (
	"os"
	"path/filepath"
	"testing"
)

// writeScript writes an executable shell script and returns its path.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-wrapper.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("writing script: %v", err)
	}
	return path
}

func TestLaunchProcess_ImmediateExitReportsError(t *testing.T) {
	// A wrapper that exits immediately (e.g. bad config / missing binary) must
	// surface as an error rather than a false-positive "started".
	bin := writeScript(t, "exit 1")

	pid, err := launchProcess(bin, t.TempDir(), "/nonexistent/config.yaml")
	if err == nil {
		t.Fatalf("launchProcess: expected error for immediately-exiting wrapper, got nil (pid=%d)", pid)
	}
	if IsProcessAlive(pid) {
		t.Errorf("launchProcess: process %d still alive after immediate exit", pid)
	}
}

func TestLaunchProcess_StaysAliveSucceeds(t *testing.T) {
	// A wrapper that stays alive past the liveness window must be reported as
	// started, with a live PID. Sleep comfortably longer than launchLivenessWait.
	bin := writeScript(t, "sleep 30")

	pid, err := launchProcess(bin, t.TempDir(), "/some/config.yaml")
	if err != nil {
		t.Fatalf("launchProcess: unexpected error for long-lived wrapper: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("launchProcess: expected positive pid, got %d", pid)
	}
	if !IsProcessAlive(pid) {
		t.Errorf("launchProcess: process %d should be alive", pid)
	}
	// Clean up the detached process.
	if err := StopServer(pid); err != nil {
		t.Logf("cleanup StopServer(%d): %v", pid, err)
	}
}
