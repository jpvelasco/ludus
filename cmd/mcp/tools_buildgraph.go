package mcp

import (
	"context"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/buildgraph"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type buildGraphInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

func registerBuildGraphTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_buildgraph",
		Description: "Generate UE5 BuildGraph XML from ludus config. Returns XML describing engine and game build stages as a DAG for Horde, UET, or other orchestrators.",
	}, handleBuildGraph)
}

func handleBuildGraph(_ context.Context, _ *mcp.CallToolRequest, _ buildGraphInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)

	bg, err := buildgraph.Generate(cfg, engineVersion)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(map[string]string{"error": err.Error()})},
			},
		}, nil, nil
	}

	data, err := bg.Marshal()
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(map[string]string{"error": err.Error()})},
			},
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}
