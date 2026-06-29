//go:build windows

package anywhere

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// launchProcess starts the wrapper binary as a background process on Windows.
// configPath is passed via the wrapper's -c flag rather than relying on the
// working directory (see the unix implementation for the rationale). The
// wrapper's output is redirected to a log file under workDir rather than
// inherited, so a long-lived server does not hold a captured stdout/stderr pipe
// open (see the unix implementation). Windows cannot reliably probe process
// liveness with only the stdlib, so this path does not perform the post-start
// liveness check; Anywhere is primarily a Linux feature and callers fall back
// to fleet status.
func launchProcess(binary, workDir, configPath string) (int, error) {
	logPath := filepath.Join(workDir, "server.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, fmt.Errorf("creating wrapper log %s: %w", logPath, err)
	}
	defer logFile.Close()

	cmd := exec.Command(binary, "-c", configPath)
	cmd.Dir = workDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting wrapper process: %w", err)
	}

	pid := cmd.Process.Pid

	if err := cmd.Process.Release(); err != nil {
		return pid, fmt.Errorf("releasing wrapper process: %w", err)
	}

	return pid, nil
}

// StopServer stops a running wrapper process by PID on Windows.
func StopServer(pid int) error {
	if pid <= 0 {
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}

	return proc.Kill()
}

// IsProcessAlive checks whether a process with the given PID is running on Windows.
// This is a best-effort check; Anywhere is primarily a Linux feature.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// On Windows, FindProcess always succeeds. We cannot reliably probe
	// without side effects using only the stdlib. Return false to indicate
	// unknown status — the caller will fall back to fleet status checks.
	return false
}
