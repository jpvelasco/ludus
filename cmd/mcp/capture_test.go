package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/buildlog"
	"github.com/jpvelasco/ludus/internal/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// childMarker is echoed by the stub setup script used by the capture tests.
const childMarker = "child-marker-747"

// --- Issue #725: concurrent tool calls must not mix output or swap streams ---

// TestToolOutputConcurrentOperationsIsolated pins #725 at the capture layer:
// two overlapping operations, each owning its toolOutput, must capture only
// their own writes. The barrier makes operation A print while operation B's
// capture is live and finish first — the interleaving that used to let the
// process-global swap steal A's output into B's capture and restore a closed
// pipe over os.Stdout.
func TestToolOutputConcurrentOperationsIsolated(t *testing.T) {
	origStdout, origStderr := os.Stdout, os.Stderr

	outA, outB := newToolOutput(), newToolOutput()
	bLive := make(chan struct{})
	aPrinted := make(chan struct{})

	var wg sync.WaitGroup
	wg.Go(func() {
		<-bLive // A prints while B's capture is live
		outA.Println("alpha")
		close(aPrinted)
	})
	wg.Go(func() {
		close(bLive)
		<-aPrinted // B outlives A, like a handler that finishes later
		outB.Println("beta")
	})
	wg.Wait()

	assertCaptureIsolated(t, "A", outA.String(), "alpha")
	assertCaptureIsolated(t, "B", outB.String(), "beta")
	if os.Stdout != origStdout || os.Stderr != origStderr {
		t.Fatal("per-operation capture must never swap os.Stdout/os.Stderr")
	}
}

// assertCaptureIsolated checks that got contains want and no other marker leaked in.
func assertCaptureIsolated(t *testing.T, name, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("capture %s = %q, want it to contain %q", name, got, want)
	}
	for _, leaked := range []string{"alpha", "beta"} {
		if leaked != want && strings.Contains(got, leaked) {
			t.Errorf("capture %s = %q, want it isolated from %q", name, got, leaked)
		}
	}
}

// TestSyncHandlersDoNotSwapProcessStreams pins #725 at the handler layer: two
// concurrent sync tool calls keep the process streams untouched (no handler
// restores another handler's closed pipe), and each response carries exactly
// its own child's output.
func TestSyncHandlersDoNotSwapProcessStreams(t *testing.T) {
	origStdout, origStderr := os.Stdout, os.Stderr
	withEngineTestConfig(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: markerSetupEngineDir(t)},
	})

	texts := make([]string, 2)
	failed := make([]error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			result, _, err := handleEngineSetup(context.Background(), nil, engineSetupInput{})
			failed[i] = err
			texts[i] = resultText(result)
		})
	}
	wg.Wait()

	for i := range 2 {
		if failed[i] != nil {
			t.Fatalf("concurrent handleEngineSetup() %d error = %v", i, failed[i])
		}
		if n := strings.Count(texts[i], childMarker); n != 1 {
			t.Errorf("response %d carries %d marker occurrences, want exactly 1 (output mixed or lost): %q", i, n, texts[i])
		}
	}
	if os.Stdout != origStdout || os.Stderr != origStderr {
		t.Fatal("sync handlers must not swap the process streams")
	}
}

// resultText extracts the first text content of a tool result without
// asserting, so it is safe to call from spawned goroutines.
func resultText(result *mcpsdk.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}

// markerSetupEngineDir returns an engine directory whose setup script echoes
// the child marker.
func markerSetupEngineDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "echo " + childMarker + "\n"
	if runtime.GOOS == "windows" {
		script = "@echo " + childMarker + "\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, engineSetupScript()), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// --- Issue #747: subprocess output must reach the tool response ---

// TestHandleEngineSetupCapturesChildOutput pins #747: the tool runner is
// constructed with the per-operation capture's writers, so the child process's
// own output (build logs, error details) lands in the response instead of
// leaking to the server's stderr.
func TestHandleEngineSetupCapturesChildOutput(t *testing.T) {
	withEngineTestConfig(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: markerSetupEngineDir(t)},
	})

	result, _, err := handleEngineSetup(context.Background(), nil, engineSetupInput{})
	if err != nil {
		t.Fatalf("handleEngineSetup() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handleEngineSetup() failed: %s", toolResultText(t, result))
	}
	if got := decodeEngineResult(t, result).Output; !strings.Contains(got, childMarker) {
		t.Errorf("response output = %q, want it to contain the child's marker", got)
	}
}

// TestNewCaptureRunner pins the runner seam of #747 and the handler wiring
// pattern for dry-run: the handler resolves the effective dry-run via
// deployToolDryRun and hands it to newCaptureRunner (the async starters pass
// an already-resolved value so the build goroutine never reads globals).
// Verbose is always on (MCP captures output) and both subprocess streams
// route into the caller's capture.
func TestNewCaptureRunner(t *testing.T) {
	origDryRun := globals.DryRun
	t.Cleanup(func() { globals.DryRun = origDryRun })

	tests := []struct {
		name       string
		inputDry   bool
		globalDry  bool
		wantDryRun bool
	}{
		{"input dry run", true, false, true},
		{"global dry run", false, true, true},
		{"both dry run", true, true, true},
		{"neither dry run", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals.DryRun = tt.globalDry
			assertCaptureRunner(t, deployToolDryRun(tt.inputDry), tt.wantDryRun)
		})
	}
}

// assertCaptureRunner checks the runner's flags and stream wiring.
func assertCaptureRunner(t *testing.T, effectiveDry, wantDryRun bool) {
	t.Helper()
	out := newToolOutput()
	r := newCaptureRunner(out, effectiveDry)
	if r == nil {
		t.Fatal("expected non-nil runner")
	}
	if !r.Verbose {
		t.Error("expected Verbose = true for MCP runner")
	}
	if r.DryRun != wantDryRun {
		t.Errorf("DryRun = %v, want %v", r.DryRun, wantDryRun)
	}
	if _, err := r.Stdout.Write([]byte("probe\n")); err != nil {
		t.Fatalf("writing to r.Stdout: %v", err)
	}
	if !strings.Contains(out.String(), "probe") {
		t.Error("runner Stdout must be wired to the per-operation capture")
	}
	if r.Stderr == nil {
		t.Error("runner Stderr must be set to the per-operation capture")
	}
}

// TestToolOutputRouting covers the emitter side of the per-operation capture:
// Write, Printf, Println, and String all land in the same buffer that the
// response is assembled from.
func TestToolOutputRouting(t *testing.T) {
	out := newToolOutput()
	if _, err := out.Write([]byte("written\n")); err != nil {
		t.Fatalf("out.Write: %v", err)
	}
	out.Printf("formatted %d\n", 7)
	out.Println("printed")

	want := "written\nformatted 7\nprinted\n"
	if got := out.String(); got != want {
		t.Errorf("out.String() = %q, want %q", got, want)
	}
	if newToolOutput().String() != "" {
		t.Error("a fresh capture must be empty")
	}
}

// --- Issues #726 and #706: build IDs, history, and log retention ---

// TestNextBuildID pins #726's collision resistance: same-type builds minted
// within the same second get distinct IDs from the process-unique counter,
// and IDs keep the readable type + timestamp shape.
func TestNextBuildID(t *testing.T) {
	now := time.Date(2026, 6, 19, 15, 4, 5, 0, time.UTC)
	first := nextBuildID(buildTypeEngineBuild, now)
	second := nextBuildID(buildTypeEngineBuild, now)
	other := nextBuildID(buildTypeGameBuild, now)

	if first == second {
		t.Fatalf("same-second same-type IDs collide: %q", first)
	}
	if first == other || second == other {
		t.Errorf("IDs must differ across types: %q %q %q", first, second, other)
	}
	for _, id := range []string{first, second} {
		if prefix := "engine_build-20260619-150405-"; !strings.HasPrefix(id, prefix) {
			t.Errorf("id %q, want prefix %q", id, prefix)
		}
	}
}

// TestBuildManagerSameSecondBuildsDistinct pins #726 end to end: two same-type
// builds started within one second get distinct IDs, both history entries
// survive, and each build keeps its own log file.
func TestBuildManagerSameSecondBuildsDistinct(t *testing.T) {
	dir := t.TempDir()
	withAsyncLogConfig(t, dir, 0)
	bm := newBuildManager()
	// Pin the build clock so both starts fall in the same second.
	bm.now = func() time.Time { return time.Date(2026, 6, 19, 15, 4, 5, 0, time.UTC) }

	first := startAndWait(t, bm, buildTypeEngineBuild)
	second := startAndWait(t, bm, buildTypeEngineBuild)

	if first == second {
		t.Fatalf("build IDs collide within the same second: %q", first)
	}
	if entries := bm.List(); len(entries) != 2 {
		t.Fatalf("List() = %d entries, want 2 (history overwritten)", len(entries))
	}
	if got := countLogs(t, dir); got != 2 {
		t.Errorf("log files = %d, want 2 (one per build, never truncated)", got)
	}
}

// TestBuildManagerFastBuildsPreserveHistory replays the #726 report: many
// fast same-type builds each keep their own ID, history entry, and log file.
func TestBuildManagerFastBuildsPreserveHistory(t *testing.T) {
	dir := t.TempDir()
	withAsyncLogConfig(t, dir, 0)
	bm := newBuildManager()

	const builds = 20
	ids := make(map[string]bool, builds)
	for range builds {
		ids[startAndWait(t, bm, buildTypeGameBuild)] = true
	}
	if len(ids) != builds {
		t.Errorf("distinct build IDs = %d, want %d", len(ids), builds)
	}
	if entries := bm.List(); len(entries) != builds {
		t.Errorf("List() entries = %d, want %d (history overwritten)", len(entries), builds)
	}
	if got := countLogs(t, dir); got != builds {
		t.Errorf("log files = %d, want %d (logs shared or truncated)", got, builds)
	}
}

// TestOpenBuildLogUsesCanonicalRetentionNaming pins #706: async build logs
// are created through buildlog's canonical naming, so they carry the ludus-
// prefix and buildlog.Prune owns them — retention actually deletes the older
// async log instead of letting every async log accumulate forever.
func TestOpenBuildLogUsesCanonicalRetentionNaming(t *testing.T) {
	dir := t.TempDir()
	// Retain 2 so opening the second log does not prune the first; the
	// explicit Prune below is what retention must own.
	withAsyncLogConfig(t, dir, 2)
	older := time.Date(2026, 6, 19, 15, 4, 5, 0, time.UTC)
	newer := older.Add(time.Hour)

	keepPath, dropPath, err := openTwoBuildLogs(dir, older, newer)
	if err != nil {
		t.Fatal(err)
	}
	if err := buildlog.Prune(dir, 1); err != nil {
		t.Fatalf("Prune error: %v", err)
	}
	if _, err := os.Stat(dropPath); !os.IsNotExist(err) {
		t.Errorf("older async log %q was not pruned (retention bypassed, #706)", dropPath)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Errorf("newer async log should survive the prune: %v", err)
	}
}

// openTwoBuildLogs opens one async log per build type, closes both (the
// retention candidates must be closed on Windows), and pins their modtimes so
// pruning is deterministic on every filesystem.
func openTwoBuildLogs(dir string, older, newer time.Time) (keepPath, dropPath string, err error) {
	oldLog := openBuildLog(buildTypeEngineBuild, older)
	newLog := openBuildLog(buildTypeGameBuild, newer)
	if oldLog == nil || newLog == nil {
		return "", "", fmt.Errorf("opening build logs in %s: nil logger", dir)
	}
	dropPath, keepPath = oldLog.Path(), newLog.Path()
	if err = closeAndStamp(oldLog, older); err != nil {
		return "", "", fmt.Errorf("stamping older log: %w", err)
	}
	return keepPath, dropPath, closeAndStamp(newLog, newer)
}

// closeAndStamp closes the log and pins its file modtime for deterministic pruning.
func closeAndStamp(lg *buildlog.Logger, when time.Time) error {
	if err := lg.Close(); err != nil {
		return err
	}
	return os.Chtimes(lg.Path(), when, when)
}

// --- shared helpers ---

// withAsyncLogConfig points async build logs at dir with the given retention
// and restores the previous logging globals.
func withAsyncLogConfig(t *testing.T, dir string, retain int) {
	t.Helper()
	origCfg, origNoLogs := globals.Cfg, globals.NoLogs
	t.Cleanup(func() { globals.Cfg, globals.NoLogs = origCfg, origNoLogs })
	globals.NoLogs = false
	globals.Cfg = &config.Config{Observability: config.ObservabilityConfig{
		Logs: config.LogsConfig{Dir: dir, RetainRuns: retain},
	}}
}

// startAndWait starts a fast build and returns its ID once it is terminal.
func startAndWait(t *testing.T, bm *buildManager, btype buildType) string {
	t.Helper()
	id, err := bm.Start(btype, func(context.Context, *syncBuffer) (any, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("Start(%s) error: %v", btype, err)
	}
	waitForTerminal(t, bm, id)
	return id
}

// countLogs returns the number of .log files in dir.
func countLogs(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			n++
		}
	}
	return n
}
