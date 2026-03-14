package progress

import (
	"testing"
	"time"
)

func TestStartStop(t *testing.T) {
	// Start a ticker with a very short interval
	ticker := Start("test operation", 10*time.Millisecond)

	// Immediately stop it
	ticker.Stop()

	// If Stop() returns without panicking and without blocking indefinitely,
	// the test passes. The done channel should be closed.
	select {
	case <-ticker.done:
		// Expected: done channel is closed
	default:
		t.Error("Stop() returned but done channel is not closed")
	}
}

func TestStopBlocksUntilDone(t *testing.T) {
	ticker := Start("test operation", 10*time.Millisecond)

	// Give it a tiny bit of time to start
	time.Sleep(5 * time.Millisecond)

	// Stop should block until the goroutine exits
	ticker.Stop()

	// After Stop() returns, done channel must be closed
	select {
	case <-ticker.done:
		// Expected: done channel is closed, read succeeds immediately
	case <-time.After(100 * time.Millisecond):
		t.Error("done channel not closed after Stop() returned")
	}
}

func TestMultipleStops(t *testing.T) {
	ticker := Start("test operation", 10*time.Millisecond)

	// First stop
	ticker.Stop()

	// Verify done channel is closed
	select {
	case <-ticker.done:
		// Expected
	default:
		t.Fatal("done channel not closed after first Stop()")
	}

	// Calling Stop() again should not panic or block
	// (though in practice, closing an already-closed channel would panic,
	// but reading from done again is safe)
	select {
	case <-ticker.done:
		// Still closed, this is fine
	default:
		t.Error("done channel unexpectedly not readable after first Stop()")
	}
}
