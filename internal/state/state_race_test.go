package state

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// TestConcurrentUpdatesKeepStateParsable pins the #552 contracts:
//   - read-modify-write helpers are serialized, so concurrent updates never
//     lose blocks entirely;
//   - the save path is atomic (temp file + rename), so a concurrent reader
//     never observes a truncated document;
//   - no temp files are left behind.
//
// Before the fix this intermittently failed with JSON parse errors from
// readers racing the truncate-then-write window.
func TestConcurrentUpdatesKeepStateParsable(t *testing.T) {
	setupTest(t)

	const (
		writers   = 8
		perWriter = 30
	)

	var writerWG sync.WaitGroup
	errs := make(chan error, writers*perWriter+64)

	stop := make(chan struct{})
	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := Load(); err != nil {
				errs <- fmt.Errorf("reader saw unusable state: %w", err)
				return
			}
		}
	}()

	for w := range writers {
		writerWG.Add(1)
		go func(w int) {
			defer writerWG.Done()
			for i := 0; i < perWriter; i++ {
				fleet := &FleetState{FleetID: fmt.Sprintf("fleet-%d-%d", w, i)}
				if err := UpdateFleet(fleet); err != nil {
					errs <- fmt.Errorf("writer %d update %d: %w", w, i, err)
					return
				}
			}
		}(w)
	}

	writerWG.Wait()
	close(stop)
	readerWG.Wait()
	close(errs)

	for err := range errs {
		t.Fatal(err)
	}

	final, err := Load()
	if err != nil {
		t.Fatalf("final Load: %v", err)
	}
	if final.Fleet == nil || final.Fleet.FleetID == "" {
		t.Errorf("final fleet block lost after %d concurrent updates", writers*perWriter)
	}

	tmps, _ := filepath.Glob(filepath.Join(stateDir, "*.tmp"))
	if len(tmps) > 0 {
		t.Errorf("temp files left behind: %v", tmps)
	}
}
