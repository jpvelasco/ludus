package mcp

import (
	"encoding/json"
	"os"

	"github.com/jpvelasco/ludus/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// Cmd starts the MCP server over stdio or optional Streamable HTTP.
var Cmd = newMCPCmd()

func newMCPCmd() *cobra.Command {
	opts := transportOptions{}
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server for AI agent orchestration",
		Long: `Starts a Model Context Protocol (MCP) server over stdio (JSON-RPC).
Use --listen 127.0.0.1:8080 to opt into Streamable HTTP at /mcp.
HTTP requires a bearer token from LUDUS_MCP_TOKEN or --token-file.
Forward the loopback port from your build host to the MCP client.
--transport http without --listen uses 127.0.0.1:8080.
Non-loopback listeners require --allow-remote.

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
		RunE: func(cmd *cobra.Command, args []string) error { return runMCP(cmd, opts) },
	}
	cmd.Flags().StringVar(&opts.transport, "transport", "stdio", "MCP transport: stdio or http")
	cmd.Flags().StringVar(&opts.listen, "listen", "", "HTTP listen address (implies http; default 127.0.0.1:8080)")
	cmd.Flags().StringVar(&opts.tokenFile, "token-file", "", "read the HTTP bearer token from this file instead of LUDUS_MCP_TOKEN")
	cmd.Flags().BoolVar(&opts.allowRemote, "allow-remote", false, "allow an HTTP listener outside loopback")
	return cmd
}

func runMCP(cmd *cobra.Command, opts transportOptions) error {
	mode, err := opts.mode(cmd.Flags().Changed("transport"), cmd.Flags().Changed("listen"))
	if err != nil {
		return err
	}
	// Save real stdout for MCP transport — all JSON-RPC goes through this.
	mcpOut := os.Stdout

	// Redirect os.Stdout to os.Stderr so stray fmt.Printf calls from
	// internal packages don't corrupt the MCP transport.
	os.Stdout = os.Stderr
	defer func() { os.Stdout = mcpOut }()

	builds = newBuildManager()

	s := newMCPServer()
	if mode == "http" {
		return runHTTP(cmd.Context(), s, opts, cmd.ErrOrStderr())
	}

	transport := &mcp.IOTransport{
		Reader: os.Stdin,
		Writer: mcpOut,
	}

	return s.Run(cmd.Context(), transport)
}

func newMCPServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "ludus", Version: version.Version}, nil)
	registerTools(s)
	return s
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
