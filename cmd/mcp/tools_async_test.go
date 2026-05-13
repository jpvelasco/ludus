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
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Error("expected IsError = true")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content item")
	}
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected *mcpsdk.TextContent, got %T", result.Content[0])
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
	}
	if result == nil {
		t.Fatal("expected non-nil result")
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
	}
	if result == nil {
		t.Fatal("expected non-nil result")
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
