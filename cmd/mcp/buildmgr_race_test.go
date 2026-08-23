package mcp

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestBuildManagerConcurrentStatusPolling drives the documented polling flow —
// status readers looping over Get/List while a build goroutine writes its
// entry's scalar fields — so the race detector fails any unsynchronized read
// of Status/EndedAt/Error/Result (the #553 regression). Builds run in
// sequence because the manager rejects duplicate same-type runs; each round
// still races multiple readers against the completing goroutine.
func TestBuildManagerConcurrentStatusPolling(t *testing.T) {
	bm := newBuildManager()

	types := []buildType{buildTypeEngineBuild, buildTypeGameBuild, buildTypeGameClient}
	rounds := 6

	for i := range rounds {
		btype := types[i%len(types)]
		id, err := bm.Start(btype, func(ctx context.Context, buf *syncBuffer) (any, error) {
			for j := range 20 {
				fmt.Fprintf(buf, "step %d\n", j)
				time.Sleep(time.Millisecond)
			}
			return map[string]string{"round": fmt.Sprint(i)}, nil
		})
		if err != nil {
			t.Fatalf("Start(round %d): %v", i, err)
		}

		var pollWG sync.WaitGroup
		for range 3 {
			pollWG.Add(1)
			go func() {
				defer pollWG.Done()
				for time.Now().Add(-3 * time.Second).Before(time.Now()) {
					snap, ok := bm.Get(id)
					if ok && snap.Status != buildStatusRunning {
						return
					}
					for _, s := range bm.List() {
						_ = s.Status
						_ = s.EndedAt
						_ = s.Error
					}
					time.Sleep(200 * time.Microsecond)
				}
			}()
		}

		pollWG.Wait()

		snap, ok := bm.Get(id)
		if !ok {
			t.Fatalf("round %d: build %q missing after wait", i, id)
		}
		if snap.Status == buildStatusRunning {
			t.Fatalf("round %d: build still running after poll window", i)
		}
		if snap.EndedAt.Before(snap.StartedAt) {
			t.Errorf("round %d: EndedAt before StartedAt", i)
		}
	}
}
