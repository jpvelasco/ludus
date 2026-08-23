package mcp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

// syncBuffer is a thread-safe bytes.Buffer implementing io.Writer.
// Used as Runner.Stdout/Stderr for per-build output capture. When sink is set
// (a per-build log file), writes are mirrored there so MCP builds are persisted
// to disk the same way CLI builds are.
type syncBuffer struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	sink io.Writer
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.sink != nil {
		_, _ = sb.sink.Write(p)
	}
	return sb.buf.Write(p)
}

// Len returns the number of bytes in the buffer.
func (sb *syncBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Len()
}

// tailLines returns the last n lines from the buffer.
func (sb *syncBuffer) tailLines(n int) string {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	data := sb.buf.String()
	lines := strings.Split(data, "\n")

	// Trim trailing empty line from final newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

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

// buildManager manages async builds with duplicate prevention.
type buildManager struct {
	mu      sync.Mutex
	entries map[string]*buildEntry
}

func newBuildManager() *buildManager {
	return &buildManager{
		entries: make(map[string]*buildEntry),
	}
}

// Start launches a new build of the given type. Returns the build ID.
// Returns an error if a build of the same type is already running.
// The fn receives a context (cancelled on Cancel()) and a *syncBuffer for output.
func (bm *buildManager) Start(btype buildType, fn func(ctx context.Context, buf *syncBuffer) (any, error)) (string, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Check for duplicate running build of same type
	for _, e := range bm.entries {
		if e.Type == btype && e.Status == buildStatusRunning {
			return "", fmt.Errorf("a %s build is already running (id: %s)", btype, e.ID)
		}
	}

	id := fmt.Sprintf("%s-%s", btype, time.Now().Format("20060102-150405"))
	ctx, cancel := context.WithCancel(context.Background())
	buf := &syncBuffer{}

	// Persist each async build's output to its own per-ID log file (the
	// long-lived MCP process can't share the CLI's single-run log). Best-effort.
	logFile := openBuildLog(id)
	if logFile != nil {
		buf.sink = logFile
	}

	entry := &buildEntry{
		ID:        id,
		Type:      btype,
		Status:    buildStatusRunning,
		StartedAt: time.Now(),
		Output:    buf,
		Cancel:    cancel,
	}
	bm.entries[id] = entry

	go func() {
		if logFile != nil {
			defer logFile.Close()
		}
		result, err := fn(ctx, buf)

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

// openBuildLog opens a per-build log file named after the build ID, honoring the
// observability config (disabled / --no-logs → nil). Best-effort: returns nil on
// any failure so a build never blocks on logging. It also prunes old logs.
func openBuildLog(id string) *os.File {
	if globals.NoLogs || globals.Cfg == nil || !globals.Cfg.Observability.Logs.IsEnabled() {
		return nil
	}
	dir := globals.Cfg.Observability.Logs.Dir
	if dir == "" {
		dir = ".ludus/logs"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	f, err := os.Create(filepath.Join(dir, id+".log"))
	if err != nil {
		return nil
	}
	_ = buildlog.Prune(dir, globals.Cfg.Observability.Logs.RetainRuns)
	return f
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
