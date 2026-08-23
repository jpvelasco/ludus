//go:build windows

package anywhere

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

// kernel32 probes for process liveness. The stdlib cannot ask whether an
// arbitrary PID is running, so IsProcessAlive goes through OpenProcess with
// PROCESS_QUERY_LIMITED_INFORMATION — no admin rights needed.
var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess = kernel32.NewProc("OpenProcess")
	procGetExitCode = kernel32.NewProc("GetExitCodeProcess")
	procCloseHandle = kernel32.NewProc("CloseHandle")
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

// launchProcess starts the wrapper binary as a background process on Windows.
// configPath is passed via the wrapper's -c flag rather than relying on the
// working directory (see the unix implementation for the rationale). The
// wrapper's output is redirected to a log file under workDir rather than
// inherited, so a long-lived server does not hold a captured stdout/stderr pipe
// open (see the unix implementation).
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

// IsProcessAlive reports whether a process with the given PID is still
// running, via OpenProcess + GetExitCodeProcess (STILL_ACTIVE). A PID that
// cannot be opened (gone, or access denied) is reported dead.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	h, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return false
	}
	defer func() { _, _, _ = procCloseHandle.Call(h) }()

	var exitCode uint32
	r, _, _ := procGetExitCode.Call(h, uintptr(unsafe.Pointer(&exitCode)))
	if r == 0 {
		return false
	}
	return exitCode == stillActive
}
