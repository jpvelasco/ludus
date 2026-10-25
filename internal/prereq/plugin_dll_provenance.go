package prereq

import "path/filepath"

// pluginDLLFix describes a set of plugin DLLs that need to be copied to
// Engine/Binaries/Win64/ for the DLL loader to find them during cook.
// Each fix is version-gated because Epic reorganizes plugin modules across
// versions — blindly copying can cause fatal class registration conflicts.
type pluginDLLFix struct {
	// name is a human-readable check name for status output.
	name string
	// description explains why this fix is needed.
	description string
	// minorVersions lists which UE5 minor versions need this fix (e.g. []int{6} for 5.6 only).
	minorVersions []int
	// pluginRelPath is the plugin's Binaries/Win64/ path relative to engine root.
	pluginRelPath string
	// dllNames are the specific DLLs to copy.
	dllNames []string
}

// knownPluginDLLFixes is the table of DLL search path issues discovered during
// cross-version E2E testing. Each entry was validated by building + cooking on
// the affected version and confirming the fix resolves the GetLastError=126.
//
// IMPORTANT: Do NOT use open-ended version ranges (e.g. minor >= 6) because
// Epic reorganizes modules across versions. The Dataflow fix for 5.6 causes
// fatal class conflicts on 5.7. Always pin to specific tested versions.
var knownPluginDLLFixes = []pluginDLLFix{
	{
		name:        "Dataflow Plugin DLLs",
		description: "HairStrandsEditor depends on Dataflow DLLs not in Engine/Binaries/Win64/",
		// 5.6 only: Epic moved Dataflow modules into Engine/Binaries/Win64/ natively in 5.7,
		// and copying the plugin versions on 5.7+ causes DataflowActor class conflicts.
		minorVersions: []int{6},
		pluginRelPath: filepath.Join("Engine", "Plugins", "Experimental", "Dataflow", "Binaries", "Win64"),
		dllNames: []string{
			"UnrealEditor-DataflowAssetTools.dll",
			"UnrealEditor-DataflowEditor.dll",
			"UnrealEditor-DataflowEnginePlugin.dll",
			"UnrealEditor-DataflowNodes.dll",
		},
	},
	{
		name:        "PlatformCrypto Plugin DLLs",
		description: "AESGCMHandlerComponent depends on PlatformCrypto DLLs not in Engine/Binaries/Win64/",
		// 5.7+: PlatformCrypto moved from engine binaries to a plugin-only location.
		// AESGCMHandlerComponent can't resolve its import dependency without the copy.
		// 5.8 keeps the plugin at the same Engine/Plugins/Experimental/PlatformCrypto
		// path (verified at the 5.8.0 release tag), so the fix applies there too;
		// omitting 8 would make cleanupStaleDLLs delete the needed DLLs on 5.8.
		minorVersions: []int{7, 8},
		dllNames: []string{
			"UnrealEditor-PlatformCrypto.dll",
			"UnrealEditor-PlatformCryptoContext.dll",
			"UnrealEditor-PlatformCryptoTypes.dll",
		},
		pluginRelPath: filepath.Join("Engine", "Plugins", "Experimental", "PlatformCrypto", "Binaries", "Win64"),
	},
}

// filenameCleanupMinor is the only engine minor version where stale cleanup
// falls back to matching the fix table's file names. The Dataflow workaround
// applied on 5.6, and copies made by ludus versions that predate the
// provenance manifest have no other record. On 5.7+ the same names belong to
// NATIVE engine binaries and must never be deleted based on the table alone
// (#746).
const filenameCleanupMinor = 6

// pluginDLLCopy records one file ludus copied into Engine/Binaries/Win64/ by
// a pluginDLLFix — the provenance for stale cleanup. Cleanup only removes
// files recorded like this, so native engine binaries that merely share a
// workaround DLL's name are never touched (#746). The JSON envelope holding
// these records lives with the platform I/O (pluginDLLManifest).
type pluginDLLCopy struct {
	Fix  string `json:"fix"`
	File string `json:"file"`
}

// fixAppliesToMinor reports whether the named fix covers the given engine
// minor version. Unknown fix names report false: ludus owns those copies, and
// no current fix needs them.
func fixAppliesToMinor(fixName string, minor int) bool {
	for _, fix := range knownPluginDLLFixes {
		if fix.name == fixName {
			return intSliceContains(fix.minorVersions, minor)
		}
	}
	return false
}

// selectStaleCopies returns the manifest entries whose owning fix no longer
// applies to the given engine minor version. Only manifest entries can be
// stale — a file ludus never copied is never a deletion candidate, whatever
// its name (#746).
func selectStaleCopies(copies []pluginDLLCopy, minor int) []pluginDLLCopy {
	var stale []pluginDLLCopy
	for _, copy := range copies {
		if !fixAppliesToMinor(copy.Fix, minor) {
			stale = append(stale, copy)
		}
	}
	return stale
}

// remainingCopies returns the manifest entries that must survive a cleanup
// pass: everything except the stale entries whose files were actually
// removed. Stale entries whose removal failed stay recorded so a later run
// reconsiders them (#746).
func remainingCopies(manifest, stale []pluginDLLCopy, removed []string) []pluginDLLCopy {
	isRemoved := map[string]bool{}
	for _, name := range removed {
		isRemoved[name] = true
	}
	var out []pluginDLLCopy
	for _, copy := range manifest {
		if isRemoved[copy.File] {
			continue
		}
		out = append(out, copy)
	}
	return out
}

// intSliceContains reports whether the list contains the value.
func intSliceContains(s []int, v int) bool {
	for _, n := range s {
		if n == v {
			return true
		}
	}
	return false
}
