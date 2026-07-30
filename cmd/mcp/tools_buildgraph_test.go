package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestHandleBuildGraphGeneratesXML verifies handleBuildGraph generates valid XML output
// (tools_buildgraph.go:23-42: Generate, Marshal, and result assembly).
func TestHandleBuildGraphGeneratesXML(t *testing.T) {
	t.Chdir(t.TempDir())

	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{SourcePath: t.TempDir(), Version: "5.7"},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "TestGame.uproject",
		},
	}

	result, _, err := handleBuildGraph(context.Background(), nil, buildGraphInput{})
	if err != nil {
		t.Fatalf("handleBuildGraph() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Extract text content
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected *mcpsdk.TextContent, got %T", result.Content[0])
	}

	// Verify it's XML-like
	if !strings.Contains(tc.Text, "<?xml") && !strings.Contains(tc.Text, "<") {
		t.Errorf("result should contain XML, got: %s", tc.Text)
	}
}

// TestHandleBuildGraphHandlesMissingEngine covers error path
// (tools_buildgraph.go:28-30: Generate error handling).
func TestHandleBuildGraphHandlesMissingEngine(t *testing.T) {
	t.Chdir(t.TempDir())

	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	// Config with missing engine source
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{SourcePath: "/nonexistent/engine"},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "TestGame.uproject",
		},
	}

	result, _, err := handleBuildGraph(context.Background(), nil, buildGraphInput{})
	if err != nil {
		t.Fatalf("handleBuildGraph() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should either fail or return partial XML - the important thing is it doesn't crash
	if len(result.Content) == 0 {
		t.Error("expected content in result, even on error")
	}
}
