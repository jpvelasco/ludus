package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CanonicalServerBinaryName returns the expected filename of the packaged
// server executable for the build configuration: Shipping and Test builds name
// the binary "<Target>-<Platform>-<Config>" (e.g. "LyraServer-Linux-Shipping"),
// while Development builds keep the bare target name (e.g. "LyraServer").
func CanonicalServerBinaryName(serverTarget, binPlatform, serverConfig string) string {
	cfg := strings.TrimSpace(serverConfig)
	switch strings.ToLower(cfg) {
	case "shipping", "test":
		return fmt.Sprintf("%s-%s-%s", serverTarget, binPlatform, cfg)
	default:
		return serverTarget
	}
}

// serverBinariesDir returns the staged Binaries directory inside a packaged
// server build: <serverBuildDir>/<packagedDirName>/Binaries/<binPlatform>.
// serverBuildDir is the platform-qualified staged build directory (e.g.
// .../PackagedServer/LinuxServer).
func serverBinariesDir(serverBuildDir, packagedDirName, binPlatform string) string {
	return filepath.Join(serverBuildDir, packagedDirName, "Binaries", binPlatform)
}

// DiscoverServerBinaryName returns the filename of the packaged server
// executable. UE stages Shipping/Test builds as
// "<Target>-<Platform>-<Config>" (e.g. "LyraServer-Linux-Shipping") while
// Development builds keep the bare target name, so the actual file is
// discovered on disk — a suffixed name wins over the bare target when both
// exist. When the Binaries directory cannot be read (e.g. before a build), the
// canonical filename for serverConfig is returned.
func DiscoverServerBinaryName(serverBuildDir, packagedDirName, serverTarget, arch, serverConfig string) string {
	binPlatform := BinariesPlatformDir(arch)
	entries, err := os.ReadDir(serverBinariesDir(serverBuildDir, packagedDirName, binPlatform))
	if err != nil {
		return CanonicalServerBinaryName(serverTarget, binPlatform, serverConfig)
	}
	prefix := serverTarget + "-" + binPlatform + "-"
	for _, e := range entries {
		if name := e.Name(); strings.HasPrefix(name, prefix) && !strings.Contains(name, ".") {
			return name
		}
	}
	return CanonicalServerBinaryName(serverTarget, binPlatform, serverConfig)
}

// DiscoverServerBinary returns the full path to the packaged server
// executable: the staged Binaries directory (see serverBinariesDir) joined
// with the discovered executable filename (see DiscoverServerBinaryName).
func DiscoverServerBinary(serverBuildDir, packagedDirName, serverTarget, arch, serverConfig string) string {
	binPlatform := BinariesPlatformDir(arch)
	name := DiscoverServerBinaryName(serverBuildDir, packagedDirName, serverTarget, arch, serverConfig)
	return filepath.Join(serverBinariesDir(serverBuildDir, packagedDirName, binPlatform), name)
}
