package config

import (
	"os"
	"path/filepath"
	"strings"
)

// isClientBinaryCandidate reports whether a file in the staged Binaries
// directory could be the client executable for the platform.
//
// On Windows the executable ends in .exe. On Linux the packaged UE executable
// has no extension at all (e.g. "LyraGame", or "LyraGame-Linux-Shipping"),
// while every sidecar UE stages alongside it contains a dot — debug symbols
// (.debug/.sym), shared objects (.so), build receipts (.target), and module
// manifests (.modules). So "no dot in the name" cleanly selects the executable
// and rejects sidecars, matching how resolveServerBinaryPath picks the server
// binary in internal/ec2fleet.
func isClientBinaryCandidate(name string, isWindows bool) bool {
	if isWindows {
		return strings.EqualFold(filepath.Ext(name), ".exe")
	}
	return !strings.Contains(name, ".")
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
		if isClientBinaryCandidate(e.Name(), isWindows) {
			matches = append(matches, e.Name())
		}
	}

	// Only trust discovery when it is unambiguous; otherwise keep the
	// conventional path so behavior never regresses.
	if len(matches) != 1 {
		return fallback
	}
	return filepath.Join(binariesDir, matches[0])
}
