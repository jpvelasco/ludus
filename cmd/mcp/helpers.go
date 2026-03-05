package mcp

import (
	"context"
	"path/filepath"

	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/deploy"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// sessionReceiver is implemented by MCP result types that can receive session info.
type sessionReceiver interface {
	setSession(id, ip string, port int)
}

func (r *deployFleetResult) setSession(id, ip string, port int) {
	r.SessionID = id
	r.SessionIP = ip
	r.SessionPort = port
}

func (r *deployStackResult) setSession(id, ip string, port int) {
	r.SessionID = id
	r.SessionIP = ip
	r.SessionPort = port
}

func (r *deployAnywhereResult) setSession(id, ip string, port int) {
	r.SessionID = id
	r.SessionIP = ip
	r.SessionPort = port
}

func (r *deployEC2Result) setSession(id, ip string, port int) {
	r.SessionID = id
	r.SessionIP = ip
	r.SessionPort = port
}

// tryCreateSession creates a game session via the target's SessionManager if
// withSession is true. Session info is written to the result via sessionReceiver.
func tryCreateSession(ctx context.Context, target deploy.Target, withSession bool, result sessionReceiver) {
	if !withSession {
		return
	}
	sm, ok := target.(deploy.SessionManager)
	if !ok {
		return
	}
	si, err := sm.CreateSession(ctx, 8)
	if err == nil && si != nil {
		result.setSession(si.SessionID, si.IPAddress, si.Port)
	}
}

// checkCacheHit returns an early MCP result if the cache stage is up to date.
// Returns nil if cache missed or disabled. The cachedResult should be a struct
// with Success/Output fields that serializes to JSON (e.g. gameBuildResult, engineResult).
func checkCacheHit(noCache bool, stage cache.StageKey, hash string, cachedResult any) *mcpsdk.CallToolResult {
	if noCache {
		return nil
	}
	c, err := cache.Load()
	if err != nil {
		return nil
	}
	if !c.IsHit(stage, hash) {
		return nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: jsonString(cachedResult)}},
	}
}

// resolveServerBuildDir determines the server build directory from config,
// matching the logic in cmd/container and cmd/pipeline.
func resolveServerBuildDir(cfg *config.Config) string {
	platformDir := config.ServerPlatformDir(cfg.Game.ResolvedArch())
	if cfg.Game.ProjectPath != "" {
		return filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "PackagedServer", platformDir)
	}
	if cfg.Engine.SourcePath != "" && cfg.Game.ProjectName == "Lyra" {
		return filepath.Join(cfg.Engine.SourcePath, "Samples", "Games", "Lyra", "PackagedServer", platformDir)
	}
	return ""
}
