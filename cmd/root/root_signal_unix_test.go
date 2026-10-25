//go:build !windows

package root

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestSignalContextCancelsRunningChildOnSIGTERM is a real-signal probe for
// #710: the root signal context must subscribe to SIGTERM as well as SIGINT,
// and a child bound to that context must be terminated when the signal
// arrives. Before the fix the stdout redirect consumed SIGTERM on its own and
// the (possibly blocked) command kept running.
func TestSignalContextCancelsRunningChildOnSIGTERM(t *testing.T) {
	ctx, stop := signalContext()
	defer stop()

	cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Skipf("starting sleep subprocess: %v", err)
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("sending SIGTERM to self: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("SIGTERM did not cancel the root signal context")
	}
	select {
	case err := <-waitDone:
		if err == nil {
			t.Error("expected the child to be killed by the cancellation, but it exited cleanly")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("child still alive after SIGTERM cancellation")
	}
}
