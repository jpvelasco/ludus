//go:build windows

package prereq

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/devrecon/ludus/internal/toolchain"
)

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
		minorVersions: []int{7},
		dllNames: []string{
			"UnrealEditor-PlatformCrypto.dll",
			"UnrealEditor-PlatformCryptoContext.dll",
			"UnrealEditor-PlatformCryptoTypes.dll",
		},
		pluginRelPath: filepath.Join("Engine", "Plugins", "Experimental", "PlatformCrypto", "Binaries", "Win64"),
	},
}

// checkPluginDLLDeps iterates through knownPluginDLLFixes and applies any
// fixes that match the current engine version. Returns one CheckResult per
// applicable fix. Also cleans up stale DLLs left by a different version's fix.
func (c *Checker) checkPluginDLLDeps() []CheckResult {
	ver, _ := toolchain.DetectEngineVersion(c.EngineSourcePath, c.EngineVersion)
	if ver == "" {
		return nil // unknown version — skip to avoid touching files unnecessarily
	}

	parts := strings.SplitN(ver, ".", 2)
	if len(parts) < 2 {
		return nil
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil
	}

	var results []CheckResult

	// First, clean up stale DLLs from other versions' fixes. For example,
	// Dataflow DLLs copied for 5.6 cause DataflowActor class conflicts on 5.7.
	results = append(results, c.cleanupStaleDLLs(minor)...)

	// Then, apply fixes for the current version.
	for _, fix := range knownPluginDLLFixes {
		if !intSliceContains(fix.minorVersions, minor) {
			continue
		}
		results = append(results, c.applyPluginDLLFix(fix))
	}
	return results
}

// cleanupStaleDLLs checks for and removes DLLs (and PDBs) in
// Engine/Binaries/Win64/ that were copied by a fix for a DIFFERENT engine
// version. Leftover files can cause class registration conflicts or load
// failures after switching UE versions.
func (c *Checker) cleanupStaleDLLs(minor int) []CheckResult {
	dstDir := filepath.Join(c.EngineSourcePath, "Engine", "Binaries", "Win64")

	var results []CheckResult
	for _, fix := range knownPluginDLLFixes {
		if intSliceContains(fix.minorVersions, minor) {
			continue // this fix is for the current version — keep its DLLs
		}
		results = append(results, c.cleanupSingleFix(dstDir, fix)...)
	}
	return results
}

// cleanupSingleFix handles stale DLL/PDB removal for one pluginDLLFix entry.
func (c *Checker) cleanupSingleFix(dstDir string, fix pluginDLLFix) []CheckResult {
	stale := findStaleFiles(dstDir, fix.dllNames)
	if len(stale) == 0 {
		return nil
	}

	if !c.Fix {
		return []CheckResult{{
			Name:   fix.name + " Cleanup",
			Passed: false,
			Message: fmt.Sprintf("found %d stale file(s) in Engine/Binaries/Win64/ from a different UE version's fix (%s); "+
				"run with --fix to remove them",
				len(stale), strings.Join(stale, ", ")),
		}}
	}

	var results []CheckResult
	var removed []string
	for _, name := range stale {
		p := filepath.Join(dstDir, name)
		if err := os.Remove(p); err != nil {
			results = append(results, CheckResult{
				Name:    fix.name + " Cleanup",
				Passed:  false,
				Message: fmt.Sprintf("failed to remove stale %s: %v", p, err),
			})
			continue
		}
		removed = append(removed, name)
	}

	if len(removed) > 0 {
		results = append(results, CheckResult{
			Name:    fix.name + " Cleanup",
			Passed:  true,
			Message: fmt.Sprintf("removed %d stale file(s) from Engine/Binaries/Win64/: %s", len(removed), strings.Join(removed, ", ")),
		})
	}
	return results
}

// findStaleFiles returns DLL and PDB file names that exist in dstDir.
func findStaleFiles(dstDir string, dllNames []string) []string {
	var stale []string
	for _, dll := range dllNames {
		if _, err := os.Stat(filepath.Join(dstDir, dll)); err == nil {
			stale = append(stale, dll)
		}
		pdb := strings.TrimSuffix(dll, ".dll") + ".pdb"
		if _, err := os.Stat(filepath.Join(dstDir, pdb)); err == nil {
			stale = append(stale, pdb)
		}
	}
	return stale
}

// applyPluginDLLFix checks and optionally copies plugin DLLs to
// Engine/Binaries/Win64/ for a single pluginDLLFix entry.
func (c *Checker) applyPluginDLLFix(fix pluginDLLFix) CheckResult {
	srcDir := filepath.Join(c.EngineSourcePath, fix.pluginRelPath)
	dstDir := filepath.Join(c.EngineSourcePath, "Engine", "Binaries", "Win64")

	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return CheckResult{
			Name:    fix.name,
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("plugin not built yet (%s); will be checked after engine build", srcDir),
		}
	}

	missing := findMissingDLLs(srcDir, dstDir, fix.dllNames)
	if len(missing) == 0 {
		return CheckResult{
			Name:    fix.name,
			Passed:  true,
			Message: fmt.Sprintf("%s present in Engine/Binaries/Win64/", fix.name),
		}
	}

	if !c.Fix {
		return CheckResult{
			Name:   fix.name,
			Passed: false,
			Message: fmt.Sprintf("missing %d DLL(s) in Engine/Binaries/Win64/ (%s); "+
				"run with --fix to copy them", len(missing), fix.description),
		}
	}

	if err := copyDLLs(srcDir, dstDir, missing); err != nil {
		return CheckResult{Name: fix.name, Passed: false, Message: err.Error()}
	}

	return CheckResult{
		Name:    fix.name,
		Passed:  true,
		Message: fmt.Sprintf("copied %d DLL(s) to Engine/Binaries/Win64/", len(missing)),
	}
}

// findMissingDLLs returns DLL names that exist in srcDir but not in dstDir.
func findMissingDLLs(srcDir, dstDir string, dllNames []string) []string {
	var missing []string
	for _, dll := range dllNames {
		if _, err := os.Stat(filepath.Join(srcDir, dll)); os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dstDir, dll)); os.IsNotExist(err) {
			missing = append(missing, dll)
		}
	}
	return missing
}

// copyDLLs copies the named DLLs from srcDir to dstDir.
func copyDLLs(srcDir, dstDir string, names []string) error {
	for _, dll := range names {
		src := filepath.Join(srcDir, dll)
		dst := filepath.Join(dstDir, dll)
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("failed to read %s: %v", src, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %v", dst, err)
		}
	}
	return nil
}

func intSliceContains(s []int, v int) bool {
	for _, n := range s {
		if n == v {
			return true
		}
	}
	return false
}
