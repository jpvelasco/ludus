package game

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveServerBinaryName discovers the packaged server executable filename
// in buildDir/packagedDir/Binaries/platformDir. Shipping and Test builds name
// their binary "<Target>-<Platform>-<Config>" (e.g.
// "LyraServer-Linux-Shipping"), while Development builds keep the bare
// target name; the suffixed name wins when both are present. executableSuffix
// is appended to candidate names on platforms that use one (".exe" on
// Windows, "" elsewhere).
//
// The bare "<Target><suffix>" name is returned whenever the binary directory
// cannot be read (empty buildDir, not-yet-packaged project) or no suffixed
// binary exists — callers that must guarantee the file is real should stat the
// result (see internal/anywhere).
func ResolveServerBinaryName(buildDir, packagedDir, platformDir, serverTarget, executableSuffix string) string {
	bareName := serverTarget + executableSuffix
	if buildDir == "" {
		return bareName
	}
	entries, err := os.ReadDir(filepath.Join(buildDir, packagedDir, "Binaries", platformDir))
	if err != nil {
		return bareName
	}
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), executableSuffix)
		// A suffixed entry must actually carry the expected suffix, and the
		// config segment must be a plain name — files like
		// "<Target>-<Platform>-Shipping.debug" are not the server binary.
		if executableSuffix != "" && name == entry.Name() {
			continue
		}
		if strings.HasPrefix(name, serverTarget+"-"+platformDir+"-") && !strings.Contains(name, ".") {
			return entry.Name()
		}
	}
	return bareName
}
