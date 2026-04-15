package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAsyncWSL2Rejection(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{Backend: "wsl2"},
	}

	tests := []struct {
		name    string
		fn      func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, any, error)
		wantErr string
	}{
		{
			"engine build start rejects wsl2",
			func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, any, error) {
				return handleEngineBuildStart(ctx, req, engineBuildStartInput{Backend: "wsl2"})
			},
			"WSL2",
		},
		{
			"game build start rejects wsl2",
			func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, any, error) {
				return handleGameBuildStart(ctx, req, gameBuildStartInput{Backend: "wsl2"})
			},
			"WSL2",
		},
		{
			"game client start rejects wsl2",
			func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, any, error) {
				return handleGameClientStart(ctx, req, gameClientStartInput{Backend: "wsl2"})
			},
			"WSL2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _, err := tt.fn(context.Background(), nil)
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
			if !strings.Contains(tc.Text, tt.wantErr) {
				t.Errorf("error message %q should contain %q", tc.Text, tt.wantErr)
			}
		})
	}
}
