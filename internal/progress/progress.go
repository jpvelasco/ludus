// Package progress provides a lightweight elapsed-time ticker for long-running
// operations. UE5 engine and game builds can run for hours with long periods of
// silence (especially during linking), so periodic status messages reassure
// users that the process is still alive.
package progress

import (
	"fmt"
	"time"
)

// Ticker prints periodic elapsed-time messages during long-running operations.
type Ticker struct {
	stop chan struct{}
	done chan struct{}
}

// Start begins printing elapsed-time messages at the given interval.
// Call Stop() when the operation completes.
func Start(operation string, interval time.Duration) *Ticker {
	t := &Ticker{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go func() {
		defer close(t.done)
		start := time.Now()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-t.stop:
				return
			case <-ticker.C:
				elapsed := time.Since(start).Round(time.Second)
				fmt.Printf("  [%s] %s still running...\n", elapsed, operation)
			}
		}
	}()
	return t
}

// Stop terminates the ticker and waits for the goroutine to exit.
func (t *Ticker) Stop() {
	close(t.stop)
	<-t.done
}
