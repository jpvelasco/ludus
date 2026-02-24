//go:build windows

package anywhere

import (
	"fmt"
	"os"
	"os/exec"
)

// launchProcess starts the wrapper binary as a background process on Windows.
func launchProcess(binary, workDir string) (int, error) {
	cmd := exec.Command(binary)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

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
