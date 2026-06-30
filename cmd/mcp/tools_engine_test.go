package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestEngineBuildInputWSL2Fields(t *testing.T) {
	input := engineBuildInput{
		Backend:   "wsl2",
		WSLNative: true,
		WSLDistro: "Ubuntu-24.04",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded engineBuildInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Backend != "wsl2" {
		t.Errorf("Backend = %q, want %q", decoded.Backend, "wsl2")
	}
	if !decoded.WSLNative {
		t.Error("expected WSLNative = true")
	}
	if decoded.WSLDistro != "Ubuntu-24.04" {
		t.Errorf("WSLDistro = %q, want %q", decoded.WSLDistro, "Ubuntu-24.04")
	}
}

func TestEngineBuildInputWSL2FieldsOmitEmpty(t *testing.T) {
	input := engineBuildInput{Backend: "native"}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	s := string(data)
	if strings.Contains(s, "wsl_native") {
		t.Errorf("wsl_native should be omitted when false, got: %s", s)
	}
	if strings.Contains(s, "wsl_distro") {
		t.Errorf("wsl_distro should be omitted when empty, got: %s", s)
	}
}

// TestEngineBuildWSL2Dispatch verifies that backend=wsl2 dispatches to the
// WSL2 handler (not the native path). On non-Windows / no-WSL2 CI, the handler
// returns a WSL2-specific error — proving the dispatch took the right branch.
func TestEngineBuildWSL2Dispatch(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{SourcePath: "/nonexistent/engine"},
	}

	result, _, err := handleEngineBuild(context.Background(), nil, engineBuildInput{
		Backend: "wsl2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// The WSL2 handler calls wsl.New() which fails on non-Windows with
	// "WSL2 is not available" — this proves the dispatch reached the WSL2
	// path, not the native path (which would fail differently).
	assertResultContains(t, result, "WSL2")
}

// TestEngineBuildInputSkipSetup verifies the skip_setup field round-trips and
// is omitted when false (the #412 MCP surface).
func TestEngineBuildInputSkipSetup(t *testing.T) {
	data, err := json.Marshal(engineBuildInput{SkipSetup: true})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !strings.Contains(string(data), "skip_setup") {
		t.Errorf("skip_setup should be present when true, got: %s", data)
	}

	off, err := json.Marshal(engineBuildInput{Backend: "native"})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if strings.Contains(string(off), "skip_setup") {
		t.Errorf("skip_setup should be omitted when false, got: %s", off)
	}
}

// TestEngineBuildSkipSetupDryRun drives the native handler with skip_setup=true
// and dry-run, asserting the Setup step is skipped (the #412 wiring reaches
// engine.BuildOptions.SkipSetup). A dry-run native build prints the commands
// without executing them, so this is safe on CI with no engine tree.
func TestEngineBuildSkipSetupDryRun(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{SourcePath: t.TempDir(), Backend: "native"},
	}

	result, _, err := handleEngineBuild(context.Background(), nil, engineBuildInput{
		Backend:   "native",
		SkipSetup: true,
		NoCache:   true,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	assertResultContains(t, result, "Skipping Setup")
}

// assertResultContains checks that a CallToolResult's text content contains substr.
func assertResultContains(t *testing.T, result *mcpsdk.CallToolResult, substr string) {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content item")
	}
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected *mcpsdk.TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(tc.Text, substr) {
		t.Errorf("result text %q should contain %q", tc.Text, substr)
	}
}
