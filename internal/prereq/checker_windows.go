//go:build windows

package prereq

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/devrecon/ludus/internal/toolchain"
)

func (c *Checker) platformChecks() []CheckResult {
	var results []CheckResult

	results = append(results, c.checkVisualStudio())

	results = append(results, c.checkMSVCToolchainConfig())

	sdkResult, sdkBuild := c.checkWindowsSDK()
	results = append(results, sdkResult)

	if sdkBuild >= 26100 && c.EngineSourcePath != "" {
		engineVersion, _ := toolchain.DetectEngineVersion(c.EngineSourcePath, c.EngineVersion)

		// C4756: UE 5.4 + SDK >= 26100 — NAN/INFINITY macros in <math.h>
		// trigger "overflow in constant arithmetic" which /WX promotes to error.
		if engineVersion == "5.4" {
			results = append(results, c.checkC4756Patch())
		}

		// NNERuntimeORT: UE 5.6 + SDK >= 26100 — INITGUID patch needed.
		if engineVersion == "" || engineVersion == "5.6" {
			results = append(results, c.checkNNERuntimeORTPatch())
		}
	}

	// Check for plugin DLL dependency issues. Certain UE versions build plugin
	// DLLs into their own Binaries/Win64/ subdirectory which is not in the DLL
	// search path when other plugins depend on them. This causes cook failures
	// with GetLastError=4551 ("Missing import"). The fix is version-specific
	// because Epic reorganizes plugin modules across versions.
	if c.EngineSourcePath != "" {
		results = append(results, c.checkPluginDLLDeps()...)
	}

	return results
}

// vswhereResult holds the fields we care about from vswhere JSON output.
type vswhereResult struct {
	DisplayName      string `json:"displayName"`
	InstallationPath string `json:"installationPath"`
}

func (c *Checker) checkVisualStudio() CheckResult {
	vswherePath := filepath.Join(
		os.Getenv("ProgramFiles(x86)"),
		"Microsoft Visual Studio", "Installer", "vswhere.exe",
	)

	if _, err := os.Stat(vswherePath); os.IsNotExist(err) {
		return CheckResult{
			Name:    "Visual Studio",
			Passed:  false,
			Message: "vswhere.exe not found; install Visual Studio with C++ workloads",
		}
	}

	// Check for any VS installation
	out, err := exec.Command(vswherePath, "-format", "json", "-utf8").Output()
	if err != nil {
		return CheckResult{
			Name:    "Visual Studio",
			Passed:  false,
			Message: fmt.Sprintf("vswhere failed: %v", err),
		}
	}

	var installs []vswhereResult
	if err := json.Unmarshal(out, &installs); err != nil || len(installs) == 0 {
		return CheckResult{
			Name:    "Visual Studio",
			Passed:  false,
			Message: "no Visual Studio installation detected",
		}
	}

	edition := installs[0].DisplayName

	// Determine which MSVC toolchain component is required for this engine version.
	// UE 5.4–5.6 need MSVC 14.38; UE 5.7+ need MSVC 14.44.
	msvcCompID := "Microsoft.VisualStudio.Component.VC.14.38.17.8.x86.x64"
	msvcCompName := "MSVC v14.38"
	if needsNewerMSVC(c.EngineSourcePath, c.EngineVersion) {
		msvcCompID = "Microsoft.VisualStudio.Component.VC.14.44.17.14.x86.x64"
		msvcCompName = "MSVC v14.44"
	}

	// Check required components. We use individual component IDs rather than
	// workload IDs (NativeDesktop, NativeGame) because workload IDs are not
	// reliably detected across all VS versions (e.g., VS 2026 doesn't report
	// them via vswhere).
	requiredComponents := []struct {
		id   string
		name string
	}{
		{"Microsoft.VisualStudio.Component.VC.Tools.x86.x64", "C++ build tools"},
		{msvcCompID, msvcCompName},
	}

	var missing []string
	for _, comp := range requiredComponents {
		compOut, compErr := exec.Command(vswherePath,
			"-requires", comp.id,
			"-format", "json", "-utf8",
		).Output()
		if compErr != nil {
			missing = append(missing, comp.name)
			continue
		}
		var compInstalls []vswhereResult
		if json.Unmarshal(compOut, &compInstalls) != nil || len(compInstalls) == 0 {
			missing = append(missing, comp.name)
		}
	}

	if len(missing) > 0 {
		if !c.Fix {
			return CheckResult{
				Name:   "Visual Studio",
				Passed: false,
				Message: fmt.Sprintf("%s found but missing components: %s; "+
					"run with --fix to install them",
					edition, strings.Join(missing, ", ")),
			}
		}

		// Auto-fix: launch VS Installer to add missing components
		setupPath := filepath.Join(
			os.Getenv("ProgramFiles(x86)"),
			"Microsoft Visual Studio", "Installer", "setup.exe",
		)
		if _, err := os.Stat(setupPath); os.IsNotExist(err) {
			return CheckResult{
				Name:   "Visual Studio",
				Passed: false,
				Message: fmt.Sprintf("VS Installer setup.exe not found at %s; install components manually via VS Installer > Modify",
					setupPath),
			}
		}

		// Build the list of missing component IDs
		missingIDs := make(map[string]string)
		for _, comp := range requiredComponents {
			for _, m := range missing {
				if comp.name == m {
					missingIDs[comp.id] = comp.name
				}
			}
		}

		// Build the setup.exe argument string. The --passive flag requires
		// the installer to run elevated, so we use PowerShell Start-Process
		// -Verb RunAs to trigger UAC. Users will see a UAC prompt.
		var setupArgs []string
		setupArgs = append(setupArgs, "modify", "--installPath", installs[0].InstallationPath)
		for id := range missingIDs {
			setupArgs = append(setupArgs, "--add", id)
		}
		setupArgs = append(setupArgs, "--passive", "--norestart")

		// Escape the argument list for PowerShell.
		psArgs := fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '%s' -Verb RunAs -Wait",
			setupPath, strings.Join(setupArgs, " "))

		cmd := exec.Command("powershell", "-NoProfile", "-Command", psArgs)
		if err := cmd.Start(); err != nil {
			return CheckResult{
				Name:    "Visual Studio",
				Passed:  false,
				Message: fmt.Sprintf("failed to launch VS Installer: %v", err),
			}
		}

		return CheckResult{
			Name:    "Visual Studio",
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("launched VS Installer (UAC prompt required) to add: %s; re-run ludus init after installation completes",
				strings.Join(missing, ", ")),
		}
	}

	return CheckResult{
		Name:    "Visual Studio",
		Passed:  true,
		Message: fmt.Sprintf("%s with C++ tools and %s", edition, msvcCompName),
	}
}

// needsNewerMSVC returns true when the engine version is 5.7 or later,
// which requires MSVC 14.44+ and would reject the 14.38 pin.
func needsNewerMSVC(sourcePath, configVersion string) bool {
	ver, _ := toolchain.DetectEngineVersion(sourcePath, configVersion)
	if ver == "" {
		return false // unknown version — 14.38 pin is the safe default
	}
	parts := strings.SplitN(ver, ".", 2)
	if len(parts) < 2 {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return minor >= 7
}

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
// the affected version and confirming the fix resolves the GetLastError=4551.
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

// msvcVersionForEngine returns the MSVC version string to pin in
// BuildConfiguration.xml based on the engine version. UE 5.4–5.6 use
// 14.38.33130; UE 5.7+ use 14.44.35207.
func msvcVersionForEngine(sourcePath, configVersion string) string {
	if needsNewerMSVC(sourcePath, configVersion) {
		return "14.44.35207"
	}
	return "14.38.33130"
}

// buildConfigXMLFor generates a BuildConfiguration.xml body. When compiler
// is non-empty (e.g. "VisualStudio2026"), a <Compiler> tag is included so
// UBT picks the correct installation instead of defaulting to VS 2022.
func buildConfigXMLFor(msvcVersion, compiler string) string {
	compilerLine := ""
	if compiler != "" {
		compilerLine = fmt.Sprintf("\n    <Compiler>%s</Compiler>", compiler)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<Configuration xmlns="https://www.unrealengine.com/BuildConfiguration">
  <WindowsPlatform>%s
    <CompilerVersion>%s</CompilerVersion>
  </WindowsPlatform>
</Configuration>
`, compilerLine, msvcVersion)
}

func (c *Checker) checkMSVCToolchainConfig() CheckResult {
	wantVersion := msvcVersionForEngine(c.EngineSourcePath, c.EngineVersion)
	versionTag := "<CompilerVersion>" + wantVersion + "</CompilerVersion>"

	// UE 5.7+ supports VS 2026 natively — set the Compiler tag so UBT
	// picks VS 2026 instead of defaulting to VS 2022 (which may not exist).
	wantCompiler := ""
	if needsNewerMSVC(c.EngineSourcePath, c.EngineVersion) {
		wantCompiler = "VisualStudio2026"
	}

	configDir := filepath.Join(os.Getenv("APPDATA"), "Unreal Engine", "UnrealBuildTool")
	configPath := filepath.Join(configDir, "BuildConfiguration.xml")

	data, err := os.ReadFile(configPath)
	if err == nil {
		content := string(data)
		hasVersion := strings.Contains(content, versionTag)
		hasCompiler := wantCompiler == "" || strings.Contains(content, "<Compiler>"+wantCompiler+"</Compiler>")
		if hasVersion && hasCompiler {
			msg := fmt.Sprintf("BuildConfiguration.xml pins MSVC %s", wantVersion)
			if wantCompiler != "" {
				msg += fmt.Sprintf(" with Compiler=%s", wantCompiler)
			}
			return CheckResult{
				Name:    "MSVC Toolchain Config",
				Passed:  true,
				Message: msg + " (" + configPath + ")",
			}
		}
	}

	if !c.Fix {
		hint := "file missing"
		if err == nil {
			hint = fmt.Sprintf("CompilerVersion not set to %s", wantVersion)
			if wantCompiler != "" && !strings.Contains(string(data), "<Compiler>"+wantCompiler+"</Compiler>") {
				hint = fmt.Sprintf("Compiler/CompilerVersion not set for %s", wantVersion)
			}
		}
		return CheckResult{
			Name:   "MSVC Toolchain Config",
			Passed: false,
			Message: fmt.Sprintf("%s at %s; run with --fix to create/update it",
				hint, configPath),
		}
	}

	// Auto-fix: write the config file with the correct MSVC version.
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return CheckResult{
			Name:    "MSVC Toolchain Config",
			Passed:  false,
			Message: fmt.Sprintf("failed to create directory %s: %v", configDir, err),
		}
	}

	if err := os.WriteFile(configPath, []byte(buildConfigXMLFor(wantVersion, wantCompiler)), 0o644); err != nil {
		return CheckResult{
			Name:    "MSVC Toolchain Config",
			Passed:  false,
			Message: fmt.Sprintf("failed to write %s: %v", configPath, err),
		}
	}

	msg := fmt.Sprintf("created %s (pins MSVC %s", configPath, wantVersion)
	if wantCompiler != "" {
		msg += ", Compiler=" + wantCompiler
	}
	msg += ")"
	return CheckResult{
		Name:    "MSVC Toolchain Config",
		Passed:  true,
		Message: msg,
	}
}

func (c *Checker) checkWindowsSDK() (CheckResult, int) {
	includeDir := filepath.Join(
		os.Getenv("ProgramFiles(x86)"),
		"Windows Kits", "10", "Include",
	)

	entries, err := os.ReadDir(includeDir)
	if err != nil {
		return CheckResult{
			Name:    "Windows SDK",
			Passed:  false,
			Message: fmt.Sprintf("cannot read %s: %v", includeDir, err),
		}, 0
	}

	// Parse version directories (e.g. "10.0.26100.0") and find the highest build number
	type sdkVer struct {
		name  string
		build int
	}
	var versions []sdkVer
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		parts := strings.Split(e.Name(), ".")
		if len(parts) < 3 {
			continue
		}
		build, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		versions = append(versions, sdkVer{name: e.Name(), build: build})
	}

	if len(versions) == 0 {
		return CheckResult{
			Name:    "Windows SDK",
			Passed:  false,
			Message: fmt.Sprintf("no Windows SDK versions found in %s", includeDir),
		}, 0
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].build > versions[j].build
	})

	highest := versions[0]

	if highest.build >= 26100 {
		return CheckResult{
			Name:    "Windows SDK",
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("SDK %s (build >= 26100 requires NNERuntimeORT patch)", highest.name),
		}, highest.build
	}

	return CheckResult{
		Name:    "Windows SDK",
		Passed:  true,
		Message: fmt.Sprintf("SDK %s", highest.name),
	}, highest.build
}

// checkC4756Patch checks and optionally fixes C4756 "overflow in constant
// arithmetic" errors in UE 5.4 source files. This occurs with MSVC 14.38 and
// Windows SDK >= 26100 because the NAN and INFINITY macros in <math.h> use
// constant expressions that trigger the warning, promoted to error by /WX.
func (c *Checker) checkC4756Patch() CheckResult {
	files := []struct {
		relPath     string
		description string
	}{
		{
			filepath.Join("Engine", "Source", "Runtime", "RenderCore", "Private", "RenderGraphPrivate.cpp"),
			"uses NAN macro",
		},
		{
			filepath.Join("Engine", "Plugins", "Runtime", "AudioSynesthesia", "Source", "AudioSynesthesiaCore", "Private", "PeakPicker.cpp"),
			"uses INFINITY macro",
		},
	}

	const pragma = "#pragma warning(disable: 4756)"

	var patched, alreadyPatched, notFound int
	var firstUnpatched string
	for _, f := range files {
		fullPath := filepath.Join(c.EngineSourcePath, f.relPath)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				notFound++
				continue
			}
			return CheckResult{
				Name:    "C4756 Patch",
				Passed:  false,
				Message: fmt.Sprintf("cannot read %s: %v", fullPath, err),
			}
		}

		content := string(data)
		if strings.Contains(content, pragma) {
			alreadyPatched++
			continue
		}

		if !c.Fix {
			if firstUnpatched == "" {
				firstUnpatched = f.relPath
			}
			continue
		}

		// Insert the pragma before the first #include
		idx := strings.Index(content, "#include")
		if idx == -1 {
			return CheckResult{
				Name:    "C4756 Patch",
				Passed:  false,
				Message: fmt.Sprintf("could not find #include in %s; patch manually", fullPath),
			}
		}

		patchedContent := content[:idx] + pragma + "\n\n" + content[idx:]
		if err := os.WriteFile(fullPath, []byte(patchedContent), 0o644); err != nil {
			return CheckResult{
				Name:    "C4756 Patch",
				Passed:  false,
				Message: fmt.Sprintf("failed to write %s: %v", fullPath, err),
			}
		}
		patched++
	}

	totalFiles := len(files)

	if notFound == totalFiles {
		return CheckResult{
			Name:    "C4756 Patch",
			Passed:  true,
			Warning: true,
			Message: "source files not found (engine may be a different version); skipped",
		}
	}

	if firstUnpatched != "" {
		return CheckResult{
			Name:   "C4756 Patch",
			Passed: false,
			Message: fmt.Sprintf("C4756 suppression missing in %s; "+
				"run with --fix to patch (required for MSVC 14.38 + SDK >= 26100)",
				firstUnpatched),
		}
	}

	if patched > 0 {
		return CheckResult{
			Name:    "C4756 Patch",
			Passed:  true,
			Message: fmt.Sprintf("patched %d file(s) (added C4756 warning suppression)", patched),
		}
	}

	return CheckResult{
		Name:    "C4756 Patch",
		Passed:  true,
		Message: "C4756 warning suppression present in affected files",
	}
}

func (c *Checker) checkNNERuntimeORTPatch() CheckResult {
	buildCSPath := filepath.Join(c.EngineSourcePath,
		"Engine", "Plugins", "NNE", "NNERuntimeORT",
		"Source", "NNERuntimeORT", "NNERuntimeORT.Build.cs",
	)

	data, err := os.ReadFile(buildCSPath)
	if err != nil {
		return CheckResult{
			Name:    "NNERuntimeORT Patch",
			Passed:  false,
			Message: fmt.Sprintf("cannot read %s: %v", buildCSPath, err),
		}
	}

	content := string(data)
	if strings.Contains(content, "INITGUID") {
		return CheckResult{
			Name:    "NNERuntimeORT Patch",
			Passed:  true,
			Message: "INITGUID definition present in NNERuntimeORT.Build.cs",
		}
	}

	if !c.Fix {
		return CheckResult{
			Name:   "NNERuntimeORT Patch",
			Passed: false,
			Message: fmt.Sprintf("INITGUID not defined in %s; "+
				"run with --fix to patch (required for Windows SDK >= 26100)",
				buildCSPath),
		}
	}

	// Auto-fix: insert PublicDefinitions.Add("INITGUID"); after ORT_USE_NEW_DXCORE_FEATURES
	marker := `PublicDefinitions.Add("ORT_USE_NEW_DXCORE_FEATURES");`
	idx := strings.Index(content, marker)
	if idx == -1 {
		return CheckResult{
			Name:   "NNERuntimeORT Patch",
			Passed: false,
			Message: fmt.Sprintf("could not find ORT_USE_NEW_DXCORE_FEATURES in %s; patch manually per UE_SOURCE_PATCHES.md",
				buildCSPath),
		}
	}

	// Find the end of the marker line so we can insert after it
	lineStart := strings.LastIndex(content[:idx], "\n") + 1
	indent := content[lineStart:idx]
	if trimmed := strings.TrimLeft(indent, " \t"); len(trimmed) > 0 {
		indent = indent[:len(indent)-len(trimmed)]
	}

	lineEnd := strings.Index(content[idx:], "\n")
	if lineEnd == -1 {
		lineEnd = len(content)
	} else {
		lineEnd += idx + 1 // include the newline
	}

	patchLine := indent + "PublicDefinitions.Add(\"INITGUID\");\n"
	patched := content[:lineEnd] + patchLine + content[lineEnd:]

	if err := os.WriteFile(buildCSPath, []byte(patched), 0o644); err != nil {
		return CheckResult{
			Name:    "NNERuntimeORT Patch",
			Passed:  false,
			Message: fmt.Sprintf("failed to write patched file: %v", err),
		}
	}

	return CheckResult{
		Name:    "NNERuntimeORT Patch",
		Passed:  true,
		Message: fmt.Sprintf("patched %s (added INITGUID definition)", buildCSPath),
	}
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

		// Build the list of stale files (DLLs + matching PDBs)
		var stale []string
		for _, dll := range fix.dllNames {
			if _, err := os.Stat(filepath.Join(dstDir, dll)); err == nil {
				stale = append(stale, dll)
			}
			pdb := strings.TrimSuffix(dll, ".dll") + ".pdb"
			if _, err := os.Stat(filepath.Join(dstDir, pdb)); err == nil {
				stale = append(stale, pdb)
			}
		}

		if len(stale) == 0 {
			continue
		}

		if !c.Fix {
			results = append(results, CheckResult{
				Name:   fix.name + " Cleanup",
				Passed: false,
				Message: fmt.Sprintf("found %d stale file(s) in Engine/Binaries/Win64/ from a different UE version's fix (%s); "+
					"run with --fix to remove them",
					len(stale), strings.Join(stale, ", ")),
			})
			continue
		}

		// Auto-fix: remove stale files
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
	}

	return results
}

// applyPluginDLLFix checks and optionally copies plugin DLLs to
// Engine/Binaries/Win64/ for a single pluginDLLFix entry.
func (c *Checker) applyPluginDLLFix(fix pluginDLLFix) CheckResult {
	srcDir := filepath.Join(c.EngineSourcePath, fix.pluginRelPath)
	dstDir := filepath.Join(c.EngineSourcePath, "Engine", "Binaries", "Win64")

	// Check if the source plugin DLLs exist at all (engine must be built first)
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return CheckResult{
			Name:    fix.name,
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("plugin not built yet (%s); will be checked after engine build", srcDir),
		}
	}

	// Check which DLLs are missing from the engine binaries dir
	var missing []string
	for _, dll := range fix.dllNames {
		srcPath := filepath.Join(srcDir, dll)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			continue // source DLL doesn't exist, skip
		}
		dstPath := filepath.Join(dstDir, dll)
		if _, err := os.Stat(dstPath); os.IsNotExist(err) {
			missing = append(missing, dll)
		}
	}

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

	// Auto-fix: copy missing DLLs from the plugin dir to Engine/Binaries/Win64/
	for _, dll := range missing {
		src := filepath.Join(srcDir, dll)
		dst := filepath.Join(dstDir, dll)
		data, err := os.ReadFile(src)
		if err != nil {
			return CheckResult{
				Name:    fix.name,
				Passed:  false,
				Message: fmt.Sprintf("failed to read %s: %v", src, err),
			}
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return CheckResult{
				Name:    fix.name,
				Passed:  false,
				Message: fmt.Sprintf("failed to write %s: %v", dst, err),
			}
		}
	}

	return CheckResult{
		Name:    fix.name,
		Passed:  true,
		Message: fmt.Sprintf("copied %d DLL(s) to Engine/Binaries/Win64/", len(missing)),
	}
}

func intSliceContains(s []int, v int) bool {
	for _, n := range s {
		if n == v {
			return true
		}
	}
	return false
}

// fixCrossCompileToolchain downloads and runs the cross-compile toolchain
// installer for the detected engine version.
func (c *Checker) fixCrossCompileToolchain(tc toolchain.CheckResult) CheckResult {
	spec := tc.Required
	if spec == nil || spec.InstallerURL == "" {
		return CheckResult{
			Name:    "Toolchain",
			Passed:  true,
			Warning: true,
			Message: "no installer URL available for this engine version",
		}
	}

	// Determine download path
	installerName := fmt.Sprintf("ludus-toolchain-%s.exe", spec.SDKVersion)
	installerPath := filepath.Join(os.TempDir(), installerName)

	// Download if not already present
	if _, err := os.Stat(installerPath); os.IsNotExist(err) {
		fmt.Printf("Downloading cross-compile toolchain (%s)...\n", spec.DirPrefix)
		fmt.Printf("  URL: %s\n", spec.InstallerURL)
		fmt.Println("  This is a large download (400-600 MB), please be patient.")

		if err := downloadFile(installerPath, spec.InstallerURL); err != nil {
			return CheckResult{
				Name:    "Toolchain",
				Passed:  false,
				Message: fmt.Sprintf("failed to download toolchain installer: %v", err),
			}
		}
		fmt.Printf("  Downloaded to %s\n", installerPath)
	} else {
		fmt.Printf("Using cached toolchain installer: %s\n", installerPath)
	}

	// Run installer via elevated PowerShell (same pattern as VS component fix)
	fmt.Println("Launching toolchain installer (UAC prompt required)...")
	psArgs := fmt.Sprintf("Start-Process -FilePath '%s' -Verb RunAs -Wait", installerPath)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", psArgs)
	if err := cmd.Run(); err != nil {
		return CheckResult{
			Name:    "Toolchain",
			Passed:  false,
			Message: fmt.Sprintf("failed to run toolchain installer: %v", err),
		}
	}

	return CheckResult{
		Name:    "Toolchain",
		Passed:  true,
		Warning: true,
		Message: fmt.Sprintf("toolchain installer completed (%s); restart your terminal for LINUX_MULTIARCH_ROOT to take effect, then re-run ludus init",
			spec.DirPrefix),
	}
}

// downloadFile downloads a URL to a local file with progress reporting.
func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url) //nolint:gosec // URL is from our hardcoded toolchain map, not user input
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	totalBytes := resp.ContentLength
	var downloaded int64

	buf := make([]byte, 32*1024)
	lastPct := -1
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			downloaded += int64(n)
			if totalBytes > 0 {
				pct := int(downloaded * 100 / totalBytes)
				if pct/10 > lastPct/10 {
					fmt.Printf("  Progress: %d%% (%d / %d MB)\n", pct, downloaded/(1024*1024), totalBytes/(1024*1024))
					lastPct = pct
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}

	return nil
}

func (c *Checker) checkDiskSpace() CheckResult {
	checkPath := c.EngineSourcePath
	if checkPath == "" {
		checkPath = "."
	}

	pathPtr, err := syscall.UTF16PtrFromString(checkPath)
	if err != nil {
		return CheckResult{
			Name:    "Disk Space",
			Passed:  false,
			Message: fmt.Sprintf("invalid path: %v", err),
		}
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	ret, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		return CheckResult{
			Name:    "Disk Space",
			Passed:  false,
			Message: fmt.Sprintf("failed to check disk space: %v", callErr),
		}
	}

	freeGB := freeBytesAvailable / (1024 * 1024 * 1024)
	const requiredGB = 100

	if freeGB < requiredGB {
		return CheckResult{
			Name:    "Disk Space",
			Passed:  false,
			Message: fmt.Sprintf("%d GB free, need %d GB", freeGB, requiredGB),
		}
	}

	return CheckResult{
		Name:    "Disk Space",
		Passed:  true,
		Message: fmt.Sprintf("%d GB free", freeGB),
	}
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func (c *Checker) checkMemory() CheckResult {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	globalMemoryStatusEx := kernel32.NewProc("GlobalMemoryStatusEx")

	var mem memoryStatusEx
	mem.Length = uint32(unsafe.Sizeof(mem))

	ret, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem)))
	if ret == 0 {
		return CheckResult{
			Name:    "Memory",
			Passed:  false,
			Message: fmt.Sprintf("failed to check memory: %v", err),
		}
	}

	totalGB := mem.TotalPhys / (1024 * 1024 * 1024)
	const requiredGB = 16

	if totalGB < requiredGB {
		return CheckResult{
			Name:    "Memory",
			Passed:  false,
			Message: fmt.Sprintf("%d GB total, need %d GB", totalGB, requiredGB),
		}
	}

	return CheckResult{
		Name:    "Memory",
		Passed:  true,
		Message: fmt.Sprintf("%d GB total", totalGB),
	}
}
