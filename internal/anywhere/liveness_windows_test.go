//go:build windows

package anywhere

import (
	"os/exec"
	"testing"
	"time"
)

// TestIsProcessAliveRealProcess pins the liveness contract against a real
// child process: alive while running, dead once it exits. The previous stub
// always returned false, making every Windows Anywhere deployment report
// not_deployed regardless of the server's actual state.
func TestIsProcessAliveRealProcess(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "ping", "-n", "3", "127.0.0.1", ">nul")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting probe process: %v", err)
	}
	pid := cmd.Process.Pid

	deadline := time.Now().Add(2 * time.Second)
	for !IsProcessAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !IsProcessAlive(pid) {
		t.Errorf("IsProcessAlive(%d) = false while the process is running", pid)
	}

	_ = cmd.Wait()
	time.Sleep(100 * time.Millisecond) // let the OS release the PID
	if IsProcessAlive(pid) {
		t.Errorf("IsProcessAlive(%d) = true after exit", pid)
	}
	if IsProcessAlive(0) || IsProcessAlive(-1) {
		t.Error("non-positive PIDs must report dead")
	}
}
