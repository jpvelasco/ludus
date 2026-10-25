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
// executableSuffix is appended to the candidate name on platforms that use
// one (".exe" on Windows, "" elsewhere).
func CanonicalServerBinaryName(serverTarget, binPlatform, serverConfig, executableSuffix string) string {
	cfg := strings.TrimSpace(serverConfig)
	switch strings.ToLower(cfg) {
	case "shipping", "test":
		return fmt.Sprintf("%s-%s-%s%s", serverTarget, binPlatform, cfg, executableSuffix)
	default:
		return serverTarget + executableSuffix
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
// exist. executableSuffix handles platforms whose executables carry one
// (".exe" on a Windows host): a suffixed entry must actually carry it, and
// files like "<Target>-<Platform>-Shipping.debug" are never the binary.
// When the Binaries directory cannot be read (e.g. before a build), the
// canonical filename for serverConfig is returned.
func DiscoverServerBinaryName(serverBuildDir, packagedDirName, serverTarget, binPlatform, serverConfig, executableSuffix string) string {
	prefix := serverTarget + "-" + binPlatform + "-"
	entries, err := os.ReadDir(serverBinariesDir(serverBuildDir, packagedDirName, binPlatform))
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if executableSuffix != "" {
				if !strings.HasSuffix(name, executableSuffix) {
					continue
				}
				name = strings.TrimSuffix(name, executableSuffix)
			}
			if strings.HasPrefix(name, prefix) && !strings.Contains(name, ".") {
				return e.Name()
			}
		}
	}
	return CanonicalServerBinaryName(serverTarget, binPlatform, serverConfig, executableSuffix)
}

// DiscoverServerBinary returns the full path to the packaged server
// executable: the staged Binaries directory (see serverBinariesDir) joined
// with the discovered executable filename (see DiscoverServerBinaryName).
func DiscoverServerBinary(serverBuildDir, packagedDirName, serverTarget, binPlatform, serverConfig, executableSuffix string) string {
	name := DiscoverServerBinaryName(serverBuildDir, packagedDirName, serverTarget, binPlatform, serverConfig, executableSuffix)
	return filepath.Join(serverBinariesDir(serverBuildDir, packagedDirName, binPlatform), name)
}
