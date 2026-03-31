package mcp

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type connectInput struct{}

type connectResult struct {
	Session        *sessionInfo `json:"session,omitempty"`
	Client         *clientInfo  `json:"client,omitempty"`
	ConnectAddress string       `json:"connect_address,omitempty"`
}

type sessionInfo struct {
	SessionID string `json:"session_id"`
	IPAddress string `json:"ip_address"`
	Port      int    `json:"port"`
}

type clientInfo struct {
	BinaryPath string `json:"binary_path"`
	Platform   string `json:"platform"`
}

func registerConnectTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_connect_info",
		Description: "Get connection info for the current game session and client build. Read-only — does not launch any processes.",
	}, handleConnectInfo)
}

func handleConnectInfo(_ context.Context, _ *mcp.CallToolRequest, _ connectInput) (*mcp.CallToolResult, any, error) {
	st, err := state.Load()
	if err != nil {
		return toolError(fmt.Sprintf("could not read state: %v", err))
	}

	var result connectResult

	if st.Session != nil {
		result.Session = &sessionInfo{
			SessionID: st.Session.SessionID,
			IPAddress: st.Session.IPAddress,
			Port:      st.Session.Port,
		}
		result.ConnectAddress = fmt.Sprintf("%s:%d", st.Session.IPAddress, st.Session.Port)
	}

	if st.Client != nil && st.Client.BinaryPath != "" {
		result.Client = &clientInfo{
			BinaryPath: st.Client.BinaryPath,
			Platform:   st.Client.Platform,
		}
	}

	return resultOK(result)
}
