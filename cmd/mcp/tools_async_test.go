package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// assertWSL2Rejected verifies that a tool call returns an error containing wantErr.
func assertWSL2Rejected(t *testing.T, result *mcpsdk.CallToolResult, err error, wantErr string) {
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

func TestAsyncWSL2RejectionEngine(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{}

	result, _, err := handleEngineBuildStart(context.Background(), nil, engineBuildStartInput{Backend: "wsl2"})
	assertWSL2Rejected(t, result, err, "WSL2")
}

func TestAsyncWSL2RejectionGame(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{}

	result, _, err := handleGameBuildStart(context.Background(), nil, gameBuildStartInput{Backend: "wsl2"})
	assertWSL2Rejected(t, result, err, "WSL2")
}

func TestAsyncWSL2RejectionClient(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{}

	result, _, err := handleGameClientStart(context.Background(), nil, gameClientStartInput{Backend: "wsl2"})
	assertWSL2Rejected(t, result, err, "WSL2")
}
