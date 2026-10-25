package globals

import (
	"path/filepath"
	"runtime"

	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
)

// ServerBuildOutput returns the output validator for a game server build: the
// packaged server directory (archive root + platform) every backend — native,
// container, and WSL2 — writes. A cache hit whose packaged output is gone is
// discarded so the build recreates it (#739). Returns nil when the project
// layout cannot be resolved, leaving hits accepted on the hash alone.
func ServerBuildOutput(cfg *config.Config) cache.OutputExists {
	return cache.DirExists(config.ResolveServerBuildDir(cfg))
}

// ClientBuildOutput returns the output validator for a game client build: the
// platform-qualified PackagedClient directory the build writes. platform is
// the requested client platform ("Linux" or "Win64"). Returns nil when the
// project layout cannot be resolved (#739).
func ClientBuildOutput(cfg *config.Config, platform string) cache.OutputExists {
	root := config.ResolveClientArchiveRoot(cfg)
	if root == "" {
		return nil
	}
	return cache.DirExists(filepath.Join(root, config.ClientPlatformDir(platform)))
}

// EngineBuildOutput returns the output validator for a native engine build:
// the host Binaries directory the engine compile produces (e.g.
// <source>/Engine/Binaries/Linux). Returns nil when the source path is
// unknown (#739).
func EngineBuildOutput(sourcePath string) cache.OutputExists {
	if sourcePath == "" {
		return nil
	}
	return cache.DirExists(engineHostBinariesDir(sourcePath))
}

// engineHostBinariesDir returns the engine Binaries subdirectory a native
// engine build populates on the host platform.
func engineHostBinariesDir(sourcePath string) string {
	dir := "Linux"
	switch runtime.GOOS {
	case "windows":
		dir = "Win64"
	case "darwin":
		dir = "Mac"
	}
	return filepath.Join(sourcePath, "Engine", "Binaries", dir)
}
