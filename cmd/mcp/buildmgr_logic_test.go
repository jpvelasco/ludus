package mcp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

// entrySnapshot reads an entry's status and result under the manager lock.
// buildEntry fields are written by the worker goroutine while holding bm.mu, so
// tests must read them under the same lock to avoid a data race (the worker has
// no happens-before edge with an unlocked read, even after a channel signal).
func entrySnapshot(bm *buildManager, id string) (status buildStatus, result any, ok bool) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	e, found := bm.entries[id]
	if !found {
		return "", nil, false
	}
	return e.Status, e.Result, true
}

// waitForStatus polls (under lock) until an entry reaches the wanted status. It
// sleeps between polls so the worker goroutine isn't starved under parallelism.
func waitForStatus(t *testing.T, bm *buildManager, id string, want buildStatus) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if status, _, ok := entrySnapshot(bm, id); ok && status == want {
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

	status, result, ok := entrySnapshot(bm, id)
	if !ok {
		t.Fatal("entry not-found for a started build")
	}
	if status != buildStatusCompleted {
		t.Errorf("status = %q, want completed", status)
	}
	if result != "result-value" {
		t.Errorf("Result = %v, want result-value", result)
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

func TestBuildManager_FailedBuildAndStaleCancel(t *testing.T) {
	bm := newBuildManager()
	id, err := bm.Start(buildTypeEngineBuild, func(context.Context, *syncBuffer) (any, error) {
		return nil, errors.New("compiler failed")
	})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	waitForStatus(t, bm, id, buildStatusFailed)

	bm.mu.Lock()
	entry := bm.entries[id]
	gotError := entry.Error
	bm.mu.Unlock()
	if gotError != "compiler failed" {
		t.Errorf("Error = %q, want compiler failed", gotError)
	}
	if err := bm.CancelBuild(id); err == nil {
		t.Fatal("expected completed build cancellation to fail")
	}
}

func TestBuildManager_ListNewestFirstAndAllowsDifferentTypes(t *testing.T) {
	bm := newBuildManager()
	release := make(chan struct{})
	start := func(kind buildType) string {
		t.Helper()
		id, err := bm.Start(kind, func(context.Context, *syncBuffer) (any, error) {
			<-release
			return kind, nil
		})
		if err != nil {
			t.Fatalf("Start(%s) error: %v", kind, err)
		}
		return id
	}

	firstID := start(buildTypeEngineBuild)
	time.Sleep(time.Millisecond)
	secondID := start(buildTypeGameBuild)
	entries := bm.List()
	if len(entries) != 2 {
		t.Fatalf("List length = %d, want 2", len(entries))
	}
	if entries[0].ID != secondID || entries[1].ID != firstID {
		t.Errorf("List IDs = [%s, %s], want [%s, %s]", entries[0].ID, entries[1].ID, secondID, firstID)
	}
	close(release)
	waitForStatus(t, bm, firstID, buildStatusCompleted)
	waitForStatus(t, bm, secondID, buildStatusCompleted)
}

func TestSyncBuffer_MirrorsSinkAndHandlesEdges(t *testing.T) {
	var sink bytes.Buffer
	sb := &syncBuffer{sink: &sink}
	if _, err := sb.Write([]byte("one\ntwo")); err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if got := sink.String(); got != "one\ntwo" {
		t.Errorf("sink = %q, want mirrored output", got)
	}
	if got := sb.tailLines(0); got != "" {
		t.Errorf("tailLines(0) = %q, want empty", got)
	}
	if got := (&syncBuffer{}).tailLines(5); got != "" {
		t.Errorf("empty tail = %q, want empty", got)
	}
}

func TestOpenBuildLog(t *testing.T) {
	origCfg, origNoLogs := globals.Cfg, globals.NoLogs
	t.Cleanup(func() { globals.Cfg, globals.NoLogs = origCfg, origNoLogs })
	t.Run("disabled", testOpenBuildLogDisabled)
	t.Run("persists output", testOpenBuildLogPersists)
	t.Run("invalid directory", testOpenBuildLogInvalidDir)
}

func testOpenBuildLogDisabled(t *testing.T) {
	globals.NoLogs = true
	globals.Cfg = &config.Config{}
	if f := openBuildLog("disabled"); f != nil {
		_ = f.Close()
		t.Fatal("openBuildLog returned a file while logs disabled")
	}
}

func testOpenBuildLogPersists(t *testing.T) {
	globals.NoLogs = false
	dir := t.TempDir()
	globals.Cfg = &config.Config{Observability: config.ObservabilityConfig{
		Logs: config.LogsConfig{Dir: dir, RetainRuns: 2},
	}}
	f := openBuildLog("game-build")
	if f == nil {
		t.Fatal("openBuildLog returned nil")
	}
	if _, err := f.WriteString("build output"); err != nil {
		t.Fatalf("WriteString error: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "game-build.log"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "build output" {
		t.Errorf("log contents = %q", data)
	}
}

func testOpenBuildLogInvalidDir(t *testing.T) {
	globals.NoLogs = false
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	globals.Cfg = &config.Config{Observability: config.ObservabilityConfig{
		Logs: config.LogsConfig{Dir: filepath.Join(file, "child")},
	}}
	if f := openBuildLog("failed"); f != nil {
		_ = f.Close()
		t.Fatal("openBuildLog should fail for invalid directory")
	}
}
