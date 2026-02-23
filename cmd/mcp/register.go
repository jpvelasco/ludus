package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools adds all ludus tools to the MCP server.
func registerTools(s *mcp.Server) {
	registerInitTool(s)
	registerStatusTool(s)
	registerEngineTools(s)
	registerGameTools(s)
	registerContainerTools(s)
	registerDeployTools(s)
	registerConnectTool(s)
}
