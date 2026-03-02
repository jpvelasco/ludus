package mcp

import (
	"encoding/json"
	"os"

	"github.com/devrecon/ludus/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// Cmd is the mcp subcommand that starts an MCP stdio server.
var Cmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for AI agent orchestration",
	Long: `Starts a Model Context Protocol (MCP) server over stdio (JSON-RPC).

AI agents can use the exposed tools to orchestrate the full UE5 dedicated
server deployment pipeline: prerequisite checks, engine builds, game builds,
container creation, deployment, and session management.

Example MCP client configuration:

  {
    "mcpServers": {
      "ludus": {
        "command": "ludus",
        "args": ["mcp"]
      }
    }
  }`,
	RunE: runMCP,
}

func runMCP(cmd *cobra.Command, args []string) error {
	// Save real stdout for MCP transport — all JSON-RPC goes through this.
	mcpOut := os.Stdout

	// Redirect os.Stdout to os.Stderr so stray fmt.Printf calls from
	// internal packages don't corrupt the MCP transport.
	os.Stdout = os.Stderr

	s := mcp.NewServer(&mcp.Implementation{
		Name:    "ludus",
		Version: version.Version,
	}, nil)

	registerTools(s)

	transport := &mcp.IOTransport{
		Reader: os.Stdin,
		Writer: mcpOut,
	}

	return s.Run(cmd.Context(), transport)
}

// jsonString marshals v to a JSON string, falling back to the error message.
func jsonString(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(data)
}

// toolError creates an error CallToolResult with a JSON error message.
// Returns three values to match the generic tool handler signature.
func toolError(msg string) (*mcp.CallToolResult, any, error) {
	result := map[string]any{
		"success": false,
		"error":   msg,
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}
