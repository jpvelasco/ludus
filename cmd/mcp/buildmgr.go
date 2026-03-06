package mcp

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
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
// Used as Runner.Stdout/Stderr for per-build output capture.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
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

// Get returns a build entry by ID.
func (bm *buildManager) Get(id string) (*buildEntry, bool) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	e, ok := bm.entries[id]
	return e, ok
}

// List returns all build entries sorted by StartedAt descending.
func (bm *buildManager) List() []*buildEntry {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	result := make([]*buildEntry, 0, len(bm.entries))
	for _, e := range bm.entries {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	return result
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
