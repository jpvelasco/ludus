package config

import (
	"os"
	"path/filepath"
	"strings"
)

// nonBinaryExts are file extensions in the Binaries directory that are never the
// client executable (debug symbols and sidecar files).
var nonBinaryExts = map[string]bool{
	".pdb":   true,
	".sym":   true,
	".debug": true,
	".so":    true,
	".dylib": true,
	".map":   true,
	".txt":   true,
}

// DiscoverClientBinary returns the path to the staged client executable in
// binariesDir. UE names the binary after the project's real client target (e.g.
// Lyra's .uproject is LyraStarterGame but its target is LyraGame), which does
// not always match the ProjectName+"Game" convention — so we discover the actual
// file rather than computing its name. fallback is the conventionally-computed
// path returned when discovery cannot identify a single executable (e.g. the
// directory does not exist yet, as in a dry run).
//
// isWindows selects the .exe matching rule: on Windows the client binary ends in
// .exe; on Linux it has no extension.
func DiscoverClientBinary(binariesDir, fallback string, isWindows bool) string {
	entries, err := os.ReadDir(binariesDir)
	if err != nil {
		return fallback
	}

	var matches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if isWindows {
			if ext != ".exe" {
				continue
			}
		} else {
			if nonBinaryExts[ext] {
				continue
			}
		}
		matches = append(matches, name)
	}

	// Only trust discovery when it is unambiguous; otherwise keep the
	// conventional path so behavior never regresses.
	if len(matches) != 1 {
		return fallback
	}
	return filepath.Join(binariesDir, matches[0])
}
