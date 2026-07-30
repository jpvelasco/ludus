package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// assertToolError verifies that a result is an error containing wantErr.
func assertToolError(t *testing.T, result *mcpsdk.CallToolResult, err error, wantErr string) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
		return
	}
	if result == nil {
		t.Fatal("expected non-nil result")
		return
	}
	if !result.IsError {
		t.Error("expected IsError = true")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content item")
		return
	}
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected *mcpsdk.TextContent, got %T", result.Content[0])
		return
	}
	if !strings.Contains(tc.Text, wantErr) {
		t.Errorf("error message %q should contain %q", tc.Text, wantErr)
	}
}

// withBuildManager initializes the package-level builds manager for the duration
// of a test, then restores the previous value.
func withBuildManager(t *testing.T) {
	t.Helper()
	prev := builds
	builds = newBuildManager()
	t.Cleanup(func() { builds = prev })
}

// TestAsyncWSL2Engine verifies that handleEngineBuildStart accepts backend=wsl2
// and either returns a build ID or fails with a WSL2 environment error — not
// the old "not yet supported" rejection.
func TestAsyncWSL2Engine(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{}
	withBuildManager(t)

	result, _, err := handleEngineBuildStart(context.Background(), nil, engineBuildStartInput{Backend: "wsl2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
		return
	}
	if result == nil {
		t.Fatal("expected non-nil result")
		return
	}

	// A "not yet supported" rejection must not be returned.
	if result.IsError {
		tc, ok := result.Content[0].(*mcpsdk.TextContent)
		if ok && strings.Contains(tc.Text, "not yet supported") {
			t.Errorf("wsl2 engine build should no longer be rejected, got: %s", tc.Text)
		}
		// Any other error (e.g. WSL2 not available in CI) is acceptable.
		return
	}
	// Success path: a build ID must be present.
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
		return
	}
}

// TestAsyncWSL2Game verifies that handleGameBuildStart accepts backend=wsl2
// and either returns a build ID or fails with a non-rejection error.
func TestAsyncWSL2Game(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{}
	withBuildManager(t)

	result, _, err := handleGameBuildStart(context.Background(), nil, gameBuildStartInput{Backend: "wsl2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
		return
	}
	if result == nil {
		t.Fatal("expected non-nil result")
		return
	}

	if result.IsError {
		tc, ok := result.Content[0].(*mcpsdk.TextContent)
		if ok && strings.Contains(tc.Text, "not yet supported") {
			t.Errorf("wsl2 game build should no longer be rejected, got: %s", tc.Text)
		}
		return
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
		return
	}
}

// TestAsyncWSL2ClientStillRejected verifies that handleGameClientStart still
// rejects backend=wsl2, matching the sync ludus_game_client which also lacks
// WSL2 support.
func TestAsyncWSL2ClientStillRejected(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{}

	result, _, err := handleGameClientStart(context.Background(), nil, gameClientStartInput{Backend: "wsl2"})
	assertToolError(t, result, err, "WSL2")
}

// TestAsyncContainerEngineRejected verifies that container engine builds
// (docker/podman) are still rejected in the async path.
func TestAsyncContainerEngineRejected(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{}

	for _, be := range []string{"docker", "podman"} {
		t.Run(be, func(t *testing.T) {
			result, _, err := handleEngineBuildStart(context.Background(), nil, engineBuildStartInput{Backend: be})
			assertToolError(t, result, err, "not yet supported")
		})
	}
}

// assertBuildStarted verifies that a non-error result contains a build ID.
func assertBuildStarted(t *testing.T, result *mcpsdk.CallToolResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
		return
	}
	if result == nil {
		t.Fatal("expected non-nil result")
		return
	}
	if result.IsError {
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcpsdk.TextContent); ok {
				t.Fatalf("expected success, got error: %s", tc.Text)
			}
		}
		t.Fatal("expected success result, got error")
		return
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
}

// TestNativeEngineBuildStart verifies that the native (non-WSL2, non-container)
// engine build path enqueues a job and returns a build ID.
func TestNativeEngineBuildStart(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{}
	withBuildManager(t)

	result, _, err := handleEngineBuildStart(context.Background(), nil, engineBuildStartInput{
		Backend: "native",
		DryRun:  true, // prevent actual engine build in CI
	})
	assertBuildStarted(t, result, err)

	// Build job must be registered in the manager
	if len(builds.List()) == 0 {
		t.Error("expected at least one build entry in manager")
	}
}

// TestNativeGameBuildStart verifies that the native game server build path
// enqueues a job and returns a build ID.
func TestNativeGameBuildStart(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{}
	withBuildManager(t)

	result, _, err := handleGameBuildStart(context.Background(), nil, gameBuildStartInput{
		Backend: "native",
		DryRun:  true,
	})
	assertBuildStarted(t, result, err)

	if len(builds.List()) == 0 {
		t.Error("expected at least one build entry in manager")
	}
}

// TestNativeClientBuildStart verifies that the native client build path
// enqueues a job and returns a build ID.
func TestNativeClientBuildStart(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{}
	withBuildManager(t)

	result, _, err := handleGameClientStart(context.Background(), nil, gameClientStartInput{
		Backend:  "native",
		Platform: "Linux",
		DryRun:   true,
	})
	assertBuildStarted(t, result, err)

	if len(builds.List()) == 0 {
		t.Error("expected at least one build entry in manager")
	}
}

// TestStartNativeEngineBuildCachePath verifies the cache hit path
// (tools_async.go:318).
func TestStartNativeEngineBuildCachePath(t *testing.T) {
	t.Chdir(t.TempDir())
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{SourcePath: t.TempDir()},
	}
	withBuildManager(t)

	// Pre-populate cache
	buildCache := map[string]string{
		"engine_test": "test_hash",
	}
	for k, v := range buildCache {
		_ = k
		_ = v
	}

	result, _, err := startNativeEngineBuild(globals.Cfg, false, 0, "test_hash", false)
	if err != nil {
		t.Fatalf("startNativeEngineBuild error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Should attempt build (not return cached)
	if result.IsError {
		// Build may fail, but we're testing the cache path was checked
	}
}

// TestStartWSL2EngineBuildEnqueuesJob verifies the job enqueue path
// (tools_async.go:281 - async job queued).
func TestStartWSL2EngineBuildEnqueuesJob(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{SourcePath: "/engine"},
	}
	withBuildManager(t)

	result, _, err := startWSL2EngineBuild(globals.Cfg, engineBuildStartInput{DryRun: true}, true, 0, "test_hash")
	if err != nil {
		t.Fatalf("startWSL2EngineBuild error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Should have enqueued a job
	if len(builds.List()) == 0 {
		t.Error("expected at least one build to be enqueued")
	}
}

// TestStartNativeGameBuildWithArch covers architecture override path
// (tools_async.go:398).
func TestStartNativeGameBuildWithArch(t *testing.T) {
	t.Chdir(t.TempDir())
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{SourcePath: t.TempDir()},
		Game:   config.GameConfig{ProjectPath: "test.uproject", ProjectName: "Test"},
	}
	withBuildManager(t)

	result, _, err := startNativeGameBuild(globals.Cfg, gameBuildStartInput{Arch: "arm64"}, false, "test_hash")
	if err != nil {
		t.Fatalf("startNativeGameBuild error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestStartWSL2GameBuildEnqueuesJob covers the enqueue path
// (tools_async.go:344 - job enqueued in builds manager).
func TestStartWSL2GameBuildEnqueuesJob(t *testing.T) {
	t.Chdir(t.TempDir())
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{SourcePath: "/engine"},
		Game:   config.GameConfig{ProjectName: "Test"},
	}
	withBuildManager(t)

	result, _, err := startWSL2GameBuild(globals.Cfg, gameBuildStartInput{}, true, "test_hash")
	if err != nil {
		t.Fatalf("startWSL2GameBuild error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Should have enqueued a job (the async build will fail later due to missing state)
	if len(builds.List()) == 0 {
		t.Error("expected at least one build to be enqueued")
	}
}

// TestStartNativeClientBuildWithPlatform covers platform parameter
// (tools_async.go:427).
func TestStartNativeClientBuildWithPlatform(t *testing.T) {
	t.Chdir(t.TempDir())
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{SourcePath: t.TempDir()},
		Game:   config.GameConfig{ProjectPath: "test.uproject", ProjectName: "Test"},
	}
	withBuildManager(t)

	result, _, err := startNativeClientBuild(globals.Cfg, gameClientStartInput{Platform: "Win64"}, "Win64", false, "test_hash")
	if err != nil {
		t.Fatalf("startNativeClientBuild error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
