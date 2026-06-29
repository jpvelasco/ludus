//go:build !windows

package anywhere

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// launchLivenessWait is how long launchProcess waits after starting the wrapper
// to confirm it stayed alive. The wrapper fails fast when it cannot load its
// config or find the game-server binary (well under a second), so this window
// catches an immediate exit without stalling a healthy launch.
const launchLivenessWait = 1500 * time.Millisecond

// launchProcess starts the wrapper binary as a detached background process.
// configPath is passed explicitly via the wrapper's -c flag rather than relying
// on the working directory: the wrapper chdir's to its own embedded output
// directory on startup and would otherwise read the stock sample config there.
func launchProcess(binary, workDir, configPath string) (int, error) {
	cmd := exec.Command(binary, "-c", configPath)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Detach process so it survives after ludus exits
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting wrapper process: %w", err)
	}

	pid := cmd.Process.Pid

	// Confirm the wrapper stayed alive. A misconfigured wrapper exits almost
	// immediately; without this check we would report a dead server as
	// "started". We detect an early exit by reaping via cmd.Wait() in a
	// goroutine and racing it against the liveness window — a signal(0) probe is
	// not enough because an exited-but-unreaped child becomes a zombie that
	// still answers signal(0) as "alive". On timeout the wrapper is healthy; we
	// leave it running (detached via Setpgid, reparented to init once ludus
	// exits) and abandon the waiter goroutine.
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	select {
	case <-exited:
		return pid, fmt.Errorf("wrapper process exited immediately after start "+
			"(PID %d); check the wrapper log above for the cause (e.g. missing game-server binary or bad config)", pid)
	case <-time.After(launchLivenessWait):
		return pid, nil
	}
}

// StopServer stops a running wrapper process by PID.
func StopServer(pid int) error {
	proc := findRunningProcess(pid)
	if proc == nil {
		return nil
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM to PID %d: %w", pid, err)
	}

	if waitForProcessExit(proc) {
		return nil
	}

	return killProcess(proc, pid)
}

func findRunningProcess(pid int) *os.Process {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil || !processAlive(proc) {
		return nil
	}
	return proc
}

func waitForProcessExit(proc *os.Process) bool {
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if !processAlive(proc) {
			return true
		}
	}
	return false
}

func killProcess(proc *os.Process, pid int) error {
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		if processAlreadyFinished(err) {
			return nil
		}
		return fmt.Errorf("sending SIGKILL to PID %d: %w", pid, err)
	}

	return nil
}

func processAlive(proc *os.Process) bool {
	return proc.Signal(syscall.Signal(0)) == nil
}

func processAlreadyFinished(err error) bool {
	return err.Error() == "os: process already finished"
}

// IsProcessAlive checks whether a process with the given PID is running.
func IsProcessAlive(pid int) bool {
	return findRunningProcess(pid) != nil
}
