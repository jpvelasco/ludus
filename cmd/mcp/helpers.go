package mcp

import (
	"path/filepath"

	"github.com/devrecon/ludus/internal/config"
)

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
