package mcp

import (
	"context"
	"sync"
	"testing"
	"time"
)

// waitForStatus polls an entry until it reaches the wanted status. It sleeps
// between polls (rather than busy-spinning) so the worker goroutine that sets
// the status isn't starved under full-package parallelism.
func waitForStatus(t *testing.T, bm *buildManager, id string, want buildStatus) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if e, ok := bm.Get(id); ok && e.Status == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("build %q did not reach status %q", id, want)
}

func TestBuildManager_StartGetComplete(t *testing.T) {
	bm := newBuildManager()
	done := make(chan struct{})
	id, err := bm.Start(buildTypeEngineBuild, func(_ context.Context, _ *syncBuffer) (any, error) {
		close(done)
		return "result-value", nil
	})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	<-done
	waitForStatus(t, bm, id, buildStatusCompleted)

	e, ok := bm.Get(id)
	if !ok {
		t.Fatal("Get returned not-found for a started build")
	}
	if e.Result != "result-value" {
		t.Errorf("Result = %v, want result-value", e.Result)
	}
}

func TestBuildManager_GetMissing(t *testing.T) {
	bm := newBuildManager()
	if _, ok := bm.Get("nope"); ok {
		t.Error("Get should return false for unknown id")
	}
}

func TestBuildManager_DuplicateRejected(t *testing.T) {
	bm := newBuildManager()
	block := make(chan struct{})
	// First build blocks so it stays "running".
	id1, err := bm.Start(buildTypeGameBuild, func(ctx context.Context, _ *syncBuffer) (any, error) {
		<-block
		return nil, nil
	})
	if err != nil {
		t.Fatalf("first Start error: %v", err)
	}

	// Second build of the same type must be rejected while the first runs.
	if _, err := bm.Start(buildTypeGameBuild, func(context.Context, *syncBuffer) (any, error) {
		return nil, nil
	}); err == nil {
		t.Error("expected duplicate-type build to be rejected")
	}

	close(block)
	waitForStatus(t, bm, id1, buildStatusCompleted)
}

func TestBuildManager_CancelBuild(t *testing.T) {
	bm := newBuildManager()

	t.Run("unknown id errors", func(t *testing.T) {
		if err := bm.CancelBuild("nope"); err == nil {
			t.Error("expected error cancelling unknown build")
		}
	})

	t.Run("running build cancels", func(t *testing.T) {
		started := make(chan struct{})
		id, err := bm.Start(buildTypeGameClient, func(ctx context.Context, _ *syncBuffer) (any, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		if err != nil {
			t.Fatalf("Start error: %v", err)
		}
		<-started
		if err := bm.CancelBuild(id); err != nil {
			t.Errorf("CancelBuild error: %v", err)
		}
		waitForStatus(t, bm, id, buildStatusCancelled)
	})
}

func TestSyncBuffer(t *testing.T) {
	var sb syncBuffer
	for _, line := range []string{"one\n", "two\n", "three\n", "four\n"} {
		if _, err := sb.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if sb.Len() == 0 {
		t.Error("Len should be non-zero after writes")
	}
	tail := sb.tailLines(2)
	if tail != "three\nfour" {
		t.Errorf("tailLines(2) = %q, want \"three\\nfour\"", tail)
	}
	// Requesting more lines than present returns all of them.
	if all := sb.tailLines(100); all != "one\ntwo\nthree\nfour" {
		t.Errorf("tailLines(100) = %q", all)
	}
}

func TestSyncBuffer_ConcurrentWrites(t *testing.T) {
	var sb syncBuffer
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			_, _ = sb.Write([]byte("x"))
		})
	}
	wg.Wait()
	if sb.Len() != 50 {
		t.Errorf("Len = %d, want 50", sb.Len())
	}
}
