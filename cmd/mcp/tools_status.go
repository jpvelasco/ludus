package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/devrecon/ludus/cmd/globals"
	internalstatus "github.com/devrecon/ludus/internal/status"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type statusInput struct{}

type statusResult struct {
	Stages []internalstatus.StageStatus `json:"stages"`
}

func registerStatusTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_status",
		Description: "Check status of all pipeline stages: engine source, engine build, game server build, container image, client build, deploy target, and game session.",
	}, handleStatus)
}

func handleStatus(ctx context.Context, _ *mcp.CallToolRequest, _ statusInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	target, err := globals.ResolveTarget(ctx, cfg, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not resolve deploy target: %v\n", err)
	}

	stages := internalstatus.CheckAll(ctx, cfg, target)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(statusResult{Stages: stages})},
		},
	}, nil, nil
}
