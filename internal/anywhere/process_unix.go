//go:build !windows

package anywhere

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// launchProcess starts the wrapper binary as a detached background process.
func launchProcess(binary, workDir string) (int, error) {
	cmd := exec.Command(binary)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Detach process so it survives after ludus exits
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting wrapper process: %w", err)
	}

	pid := cmd.Process.Pid

	// Release the process so it continues after we return
	if err := cmd.Process.Release(); err != nil {
		return pid, fmt.Errorf("releasing wrapper process: %w", err)
	}

	return pid, nil
}

// StopServer stops a running wrapper process by PID.
func StopServer(pid int) error {
	if pid <= 0 {
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}

	// Check if process is alive
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return nil
	}

	// Send SIGTERM
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM to PID %d: %w", pid, err)
	}

	// Wait briefly for graceful shutdown
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			return nil
		}
	}

	// Force kill if still alive
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		if err.Error() == "os: process already finished" {
			return nil
		}
		return fmt.Errorf("sending SIGKILL to PID %d: %w", pid, err)
	}

	return nil
}

// IsProcessAlive checks whether a process with the given PID is running.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
