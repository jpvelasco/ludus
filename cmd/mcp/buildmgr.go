package mcp

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/buildlog"
)

// buildType identifies the kind of build.
type buildType string

const (
	buildTypeEngineBuild buildType = "engine_build"
	buildTypeGameBuild   buildType = "game_build"
	buildTypeGameClient  buildType = "game_client"
)

// buildStatus tracks the lifecycle of a build.
type buildStatus string

const (
	buildStatusRunning   buildStatus = "running"
	buildStatusCompleted buildStatus = "completed"
	buildStatusFailed    buildStatus = "failed"
	buildStatusCancelled buildStatus = "cancelled"
)

// buildEntry tracks one async build.
type buildEntry struct {
	ID        string
	Type      buildType
	Status    buildStatus
	StartedAt time.Time
	EndedAt   time.Time
	Result    any
	Error     string
	Output    *syncBuffer
	Cancel    context.CancelFunc
}

// buildIDSeq is the process-unique monotonic counter embedded in every build
// ID, so two builds of the same type started within the same second can never
// alias (#726).
var buildIDSeq atomic.Int64

// nextBuildID mints a collision-resistant build ID from the build type, a
// second-resolution timestamp, and a process-unique sequence number. A
// timestamp alone let a completed build's ID be reused by an immediate
// restart within the same second, replacing the map entry and truncating
// its still-referenced log (#726).
func nextBuildID(btype buildType, now time.Time) string {
	seq := buildIDSeq.Add(1)
	return fmt.Sprintf("%s-%s-%04d", btype, now.Format("20060102-150405"), seq)
}

// buildManager manages async builds with duplicate prevention.
type buildManager struct {
	mu      sync.Mutex
	entries map[string]*buildEntry
	// now supplies the build clock for ID allocation and StartedAt. It is a
	// field so tests can pin two builds to the same second deterministically.
	now func() time.Time
}

func newBuildManager() *buildManager {
	return &buildManager{
		entries: make(map[string]*buildEntry),
		now:     time.Now,
	}
}

// runningBuildID returns the ID of a currently running build of the given
// type, if any. Callers must hold bm.mu.
func (bm *buildManager) runningBuildID(btype buildType) (string, bool) {
	for _, e := range bm.entries {
		if e.Type == btype && e.Status == buildStatusRunning {
			return e.ID, true
		}
	}
	return "", false
}

// newBuildEntry assembles the tracked entry for a freshly started build.
func newBuildEntry(btype buildType, id string, started time.Time, buf *syncBuffer, cancel context.CancelFunc) *buildEntry {
	return &buildEntry{
		ID:        id,
		Type:      btype,
		Status:    buildStatusRunning,
		StartedAt: started,
		Output:    buf,
		Cancel:    cancel,
	}
}

// Start launches a new build of the given type. Returns the build ID.
// Returns an error if a build of the same type is already running.
// The fn receives a context (cancelled on Cancel()) and a *syncBuffer for output.
func (bm *buildManager) Start(btype buildType, fn func(ctx context.Context, buf *syncBuffer) (any, error)) (string, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Reject a duplicate running build of the same type.
	if id, running := bm.runningBuildID(btype); running {
		return "", fmt.Errorf("a %s build is already running (id: %s)", btype, id)
	}

	now := bm.now()
	id := nextBuildID(btype, now)
	ctx, cancel := context.WithCancel(context.Background())
	buf := &syncBuffer{}

	// Persist each async build's output to its own log file (the long-lived
	// MCP process can't share the CLI's single-run log). Naming goes through
	// buildlog.New so async logs carry the canonical ludus- prefix and are
	// therefore owned by the same retention policy as CLI logs (#706).
	// Best-effort: a build never blocks on logging.
	logFile := openBuildLog(btype, now)
	if logFile != nil {
		buf.sink = logFile.Writer()
	}

	entry := newBuildEntry(btype, id, now, buf, cancel)
	bm.entries[id] = entry

	go func() {
		result, err := fn(ctx, buf)
		// Close the per-build log before flipping the entry to a terminal
		// status, so a caller that observes the terminal status (e.g. a test
		// cleaning up its temp dir on Windows) knows the file handle is gone.
		if logFile != nil {
			_ = logFile.Close()
		}

		bm.mu.Lock()
		defer bm.mu.Unlock()

		entry.EndedAt = time.Now()
		switch {
		case ctx.Err() == context.Canceled:
			entry.Status = buildStatusCancelled
			entry.Error = "build cancelled"
		case err != nil:
			entry.Status = buildStatusFailed
			entry.Error = err.Error()
		default:
			entry.Status = buildStatusCompleted
			entry.Result = result
		}
	}()

	return id, nil
}

// openBuildLog opens a per-build log file for an async build, honoring the
// observability config (disabled / --no-logs → nil). The file is created by
// buildlog.New — the single owner of log naming — so it carries the canonical
// ludus- prefix and is never truncated, and buildlog.Prune can see it (#706).
// Best-effort: returns nil on any failure so a build never blocks on logging.
// It also prunes old logs.
func openBuildLog(btype buildType, now time.Time) *buildlog.Logger {
	if globals.NoLogs || globals.Cfg == nil || !globals.Cfg.Observability.Logs.IsEnabled() {
		return nil
	}
	dir := globals.Cfg.Observability.Logs.Dir
	if dir == "" {
		dir = ".ludus/logs"
	}
	lg, err := buildlog.New(dir, string(btype), now)
	if err != nil {
		return nil
	}
	_ = buildlog.Prune(dir, globals.Cfg.Observability.Logs.RetainRuns)
	return lg
}

// buildSnapshot is an immutable copy of an entry's scalar state. Output is
// shared with the entry but is itself thread-safe (syncBuffer). Get and List
// return snapshots so readers never touch entry scalars outside the manager
// lock while a build goroutine writes them.
type buildSnapshot struct {
	ID        string
	Type      buildType
	Status    buildStatus
	StartedAt time.Time
	EndedAt   time.Time
	Result    any
	Error     string
	Output    *syncBuffer
}

// persistFailedWarn formats the warning appended to a successful tool result
// when writing .ludus state fails, so handlers never report success while the
// recorded state silently went stale (#543).
func persistFailedWarn(err error) string {
	return "\nWarning: operation succeeded but .ludus state could not be updated: " + err.Error()
}

// Get returns a snapshot of the build entry with the given ID.
func (bm *buildManager) Get(id string) (buildSnapshot, bool) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	e, ok := bm.entries[id]
	if !ok {
		return buildSnapshot{}, false
	}
	return e.snapshot(), true
}

// List returns snapshots of all builds sorted by StartedAt descending.
func (bm *buildManager) List() []buildSnapshot {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	result := make([]buildSnapshot, 0, len(bm.entries))
	for _, e := range bm.entries {
		result = append(result, e.snapshot())
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	return result
}

// snapshot copies the entry's scalar state. Must be called under bm.mu.
func (e *buildEntry) snapshot() buildSnapshot {
	return buildSnapshot{
		ID:        e.ID,
		Type:      e.Type,
		Status:    e.Status,
		StartedAt: e.StartedAt,
		EndedAt:   e.EndedAt,
		Result:    e.Result,
		Error:     e.Error,
		Output:    e.Output,
	}
}

// CancelBuild cancels a running build by ID.
func (bm *buildManager) CancelBuild(id string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	e, ok := bm.entries[id]
	if !ok {
		return fmt.Errorf("build %q not found", id)
	}
	if e.Status != buildStatusRunning {
		return fmt.Errorf("build %q is not running (status: %s)", id, e.Status)
	}
	e.Cancel()
	return nil
}
