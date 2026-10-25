package config

import (
	"os"
	"path/filepath"
	"strings"
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

// PackagedDirName resolves the name of the packaged content directory UE
// creates inside a staged build (e.g. .../LinuxServer/<dir>/Binaries). Unreal
// names this directory after the .uproject filename, NOT after the build
// target or any configured project name — so a project shipping
// LyraStarterGame6.uproject with a LyraServer target, or ExampleContent.uproject
// with projectName=Example, packages content under the .uproject basename.
// projectPath wins when set; projectName follows (for in-engine projects such
// as the Lyra sample); "Lyra" is the final fallback. This is the single owner
// of that resolution — builders and deployers must derive their staged
// directory from it rather than repeating the fallback chain.
func PackagedDirName(projectPath, projectName string) string {
	if projectPath != "" {
		base := filepath.Base(projectPath)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	if projectName != "" {
		return projectName
	}
	return "Lyra"
}

// ResolvedPackagedDirName returns the packaged content directory name for the
// configured project via PackagedDirName.
func (g *GameConfig) ResolvedPackagedDirName() string {
	return PackagedDirName(g.ProjectPath, g.ProjectName)
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

// ResolveClientArchiveRoot returns the PackagedClient archive root, mirroring
// the native client builder's directory layout (the packaged client is written
// next to the project as a sibling of PackagedServer). Container client
// assemblies hand this to the Docker game builder so client builds archive
// under PackagedClient instead of the server root (#750).
func ResolveClientArchiveRoot(cfg *Config) string {
	if cfg.Game.ProjectPath != "" {
		return filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "PackagedClient")
	}
	if cfg.Engine.SourcePath != "" && cfg.Game.ProjectName == "Lyra" {
		return filepath.Join(cfg.Engine.SourcePath, "Samples", "Games", "Lyra", "PackagedClient")
	}
	return ""
}
