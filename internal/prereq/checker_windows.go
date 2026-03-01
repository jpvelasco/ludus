//go:build windows

package prereq

import (
	"encoding/json"
	"fmt"
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
		// Only check/apply the NNERuntimeORT patch for engine 5.6 or when
		// the version cannot be determined (safe default).
		engineVersion, _ := toolchain.DetectEngineVersion(c.EngineSourcePath, c.EngineVersion)
		if engineVersion == "" || engineVersion == "5.6" {
			results = append(results, c.checkNNERuntimeORTPatch())
		}
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
