package mcp

import (
	"context"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type initInput struct {
	Fix bool `json:"fix,omitempty" jsonschema:"Auto-fix issues where possible"`
}

type initResult struct {
	Checks  []prereq.CheckResult `json:"checks"`
	Passed  int                  `json:"passed"`
	Failed  int                  `json:"failed"`
	Warned  int                  `json:"warned"`
	Success bool                 `json:"success"`
	Output  string               `json:"output,omitempty"`
}

func registerInitTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_init",
		Description: "Validate prerequisites for the UE5 dedicated server pipeline. Checks OS, engine source, toolchain, game content, Docker, AWS CLI, Git, Go, disk space, and RAM.",
	}, handleInit)
}

func handleInit(_ context.Context, _ *mcp.CallToolRequest, input initInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	var result initResult

	captured, _ := withCapture(func() error {
		checker := prereq.NewChecker(
			cfg.Engine.SourcePath,
			cfg.Engine.Version,
			input.Fix,
			&cfg.Game,
		)
		result.Checks = checker.RunAll()
		return nil
	})

	result.Output = captured.Stdout + captured.Stderr

	for _, c := range result.Checks {
		switch {
		case c.Passed && c.Warning:
			result.Warned++
		case c.Passed:
			result.Passed++
		default:
			result.Failed++
		}
	}
	result.Success = result.Failed == 0

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}
