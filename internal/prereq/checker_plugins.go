//go:build windows

package prereq

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jpvelasco/ludus/internal/toolchain"
)

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

// cleanupStaleDLLs removes workaround DLLs that no longer apply to the
// current engine version. Files are deletion candidates only when the
// provenance manifest records ludus copying them — native engine binaries
// (e.g. the Dataflow DLLs Epic ships in Engine/Binaries/Win64/ from 5.7 on)
// merely sharing a workaround DLL's name are never touched (#746). Filename
// matching against the fix table remains only for engine 5.6, where the
// workaround actually applied and pre-manifest copies have no record.
func (c *Checker) cleanupStaleDLLs(minor int) []CheckResult {
	dstDir := filepath.Join(c.EngineSourcePath, "Engine", "Binaries", "Win64")

	var results []CheckResult
	results = append(results, c.cleanupManifestCopies(dstDir, minor)...)
	if minor == filenameCleanupMinor {
		for _, fix := range knownPluginDLLFixes {
			if intSliceContains(fix.minorVersions, minor) {
				continue // this fix is for the current version — keep its DLLs
			}
			results = append(results, c.cleanupSingleFix(dstDir, fix)...)
		}
	}
	return results
}

// cleanupManifestCopies removes the files the provenance manifest attributes
// to a fix that no longer applies to this engine version (#746).
func (c *Checker) cleanupManifestCopies(dstDir string, minor int) []CheckResult {
	manifest := readPluginDLLManifest(dstDir)
	stale := selectStaleCopies(manifest.Copies, minor)
	if len(stale) == 0 {
		return nil
	}

	names := copiedFileNames(stale)
	if !c.Fix {
		return []CheckResult{{
			Name:   "Plugin DLL Cleanup",
			Passed: false,
			Message: fmt.Sprintf("found %d stale copied file(s) in Engine/Binaries/Win64/ from a different UE version's fix (%s); "+
				"run with --fix to remove them", len(stale), strings.Join(names, ", ")),
		}}
	}

	removed, results := c.removeStaleCopies(dstDir, stale)
	if len(removed) > 0 {
		results = append(results, CheckResult{
			Name:    "Plugin DLL Cleanup",
			Passed:  true,
			Message: fmt.Sprintf("removed %d stale copied file(s) from Engine/Binaries/Win64/: %s", len(removed), strings.Join(removed, ", ")),
		})
	}
	if err := writePluginDLLManifest(dstDir, remainingCopies(manifest.Copies, stale, removed)); err != nil {
		results = append(results, CheckResult{
			Name:    "Plugin DLL Cleanup",
			Passed:  false,
			Message: err.Error(),
		})
	}
	return results
}

// removeStaleCopies deletes the stale copies, returning the file names removed
// plus a failure CheckResult for each file that could not be removed.
func (c *Checker) removeStaleCopies(dstDir string, stale []pluginDLLCopy) ([]string, []CheckResult) {
	var removed []string
	var results []CheckResult
	for _, copy := range stale {
		p := filepath.Join(dstDir, copy.File)
		if err := os.Remove(p); err != nil {
			results = append(results, CheckResult{
				Name:    "Plugin DLL Cleanup",
				Passed:  false,
				Message: fmt.Sprintf("failed to remove stale %s: %v", p, err),
			})
			continue
		}
		removed = append(removed, copy.File)
	}
	return removed, results
}

// copiedFileNames lists the file names of the manifest entries.
func copiedFileNames(copies []pluginDLLCopy) []string {
	names := make([]string, len(copies))
	for i, copy := range copies {
		names[i] = copy.File
	}
	return names
}

// pluginDLLManifest is the on-disk provenance record (written next to the
// copied DLLs in Engine/Binaries/Win64/) listing every plugin DLL file ludus
// copied, so stale cleanup can distinguish ludus-owned copies from native
// engine binaries (#746).
type pluginDLLManifest struct {
	Copies []pluginDLLCopy `json:"copies"`
}

// pluginDLLManifestName is the provenance manifest file name, written next to
// the copied DLLs in Engine/Binaries/Win64/.
const pluginDLLManifestName = ".ludus-plugin-dlls.json"

// pluginDLLManifestPath returns the manifest location inside the binaries dir.
func pluginDLLManifestPath(dstDir string) string {
	return filepath.Join(dstDir, pluginDLLManifestName)
}

// readPluginDLLManifest loads the provenance manifest. A missing, unreadable,
// or corrupt manifest reads as empty — without provenance, cleanup removes
// nothing (#746).
func readPluginDLLManifest(dstDir string) pluginDLLManifest {
	var manifest pluginDLLManifest
	data, err := os.ReadFile(pluginDLLManifestPath(dstDir))
	if err != nil {
		return manifest
	}
	_ = json.Unmarshal(data, &manifest)
	return manifest
}

// writePluginDLLManifest rewrites the manifest with the given entries,
// removing the manifest file itself when none remain.
func writePluginDLLManifest(dstDir string, copies []pluginDLLCopy) error {
	if len(copies) == 0 {
		if err := os.Remove(pluginDLLManifestPath(dstDir)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing plugin DLL manifest: %w", err)
		}
		return nil
	}
	data, err := json.Marshal(pluginDLLManifest{Copies: copies})
	if err != nil {
		return fmt.Errorf("encoding plugin DLL manifest: %w", err)
	}
	if err := os.WriteFile(pluginDLLManifestPath(dstDir), data, 0o644); err != nil {
		return fmt.Errorf("writing plugin DLL manifest: %w", err)
	}
	return nil
}

// cleanupSingleFix handles stale DLL/PDB removal for one pluginDLLFix entry
// via filename matching (5.6 fallback only — see cleanupStaleDLLs).
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
// Engine/Binaries/Win64/ for a single pluginDLLFix entry. Copies are recorded
// in the provenance manifest so later cleanup can distinguish ludus-owned
// files from native engine binaries (#746).
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

	if err := copyDLLs(fix.name, srcDir, dstDir, missing); err != nil {
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

// copyDLLs copies the named DLLs from srcDir to dstDir, recording each
// successful copy in the provenance manifest as it goes (#746).
func copyDLLs(fixName, srcDir, dstDir string, names []string) error {
	manifest := readPluginDLLManifest(dstDir)
	for _, dll := range names {
		if err := copyDLL(srcDir, dstDir, dll); err != nil {
			return err
		}
		manifest.Copies = append(manifest.Copies, pluginDLLCopy{Fix: fixName, File: dll})
		if err := writePluginDLLManifest(dstDir, manifest.Copies); err != nil {
			return err
		}
	}
	return nil
}

// copyDLL copies a single DLL from srcDir to dstDir.
func copyDLL(srcDir, dstDir, dll string) error {
	src := filepath.Join(srcDir, dll)
	dst := filepath.Join(dstDir, dll)
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %v", dst, err)
	}
	return nil
}
