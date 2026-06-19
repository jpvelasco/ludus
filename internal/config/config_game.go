package config

import (
	"os"
	"path/filepath"
)

// ResolveProjectPath fills in ProjectPath if empty by checking known locations.
// For Lyra, it checks <engineSourcePath>/Samples/Games/Lyra/Lyra.uproject.
// For other projects, the user must set game.projectPath explicitly.
func (g *GameConfig) ResolveProjectPath(engineSourcePath string) {
	if g.ProjectPath != "" || engineSourcePath == "" {
		return
	}
	if g.ProjectName == "Lyra" || g.ProjectName == "" {
		candidate := filepath.Join(engineSourcePath, "Samples", "Games", "Lyra", "Lyra.uproject")
		if _, err := os.Stat(candidate); err == nil {
			g.ProjectPath = candidate
		}
	}
}

// ResolvedServerTarget returns the server target name, defaulting to ProjectName + "Server".
func (g *GameConfig) ResolvedServerTarget() string {
	if g.ServerTarget != "" {
		return g.ServerTarget
	}
	return g.ProjectName + "Server"
}

// ResolvedClientTarget returns the client target name, defaulting to ProjectName + "Game".
func (g *GameConfig) ResolvedClientTarget() string {
	if g.ClientTarget != "" {
		return g.ClientTarget
	}
	return g.ProjectName + "Game"
}

// ResolvedGameTarget returns the default game target name, defaulting to ProjectName + "Game".
func (g *GameConfig) ResolvedGameTarget() string {
	if g.GameTarget != "" {
		return g.GameTarget
	}
	return g.ProjectName + "Game"
}

// ResolvedArch returns the normalized architecture, defaulting to "amd64".
func (g *GameConfig) ResolvedArch() string {
	return NormalizeArch(g.Arch)
}

// ResolveServerBuildDir determines the packaged server build directory from
// config, including the platform subdirectory (e.g. .../PackagedServer/LinuxServer).
func ResolveServerBuildDir(cfg *Config) string {
	root := ResolveServerArchiveRoot(cfg)
	if root == "" {
		return ""
	}
	return filepath.Join(root, ServerPlatformDir(cfg.Game.ResolvedArch()))
}

// ResolveServerArchiveRoot returns the PackagedServer archive root (without the
// platform subdirectory). This is the value passed as the Docker game builder's
// archive directory, since it appends the platform subdirectory itself — passing
// the platform-qualified path would double it (.../PackagedServer/LinuxServer/LinuxServer).
func ResolveServerArchiveRoot(cfg *Config) string {
	if cfg.Game.ProjectPath != "" {
		return filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "PackagedServer")
	}
	if cfg.Engine.SourcePath != "" && cfg.Game.ProjectName == "Lyra" {
		return filepath.Join(cfg.Engine.SourcePath, "Samples", "Games", "Lyra", "PackagedServer")
	}
	return ""
}
