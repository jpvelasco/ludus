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
)

func (c *Checker) platformChecks() []CheckResult {
	var results []CheckResult

	results = append(results, c.checkVisualStudio())
	results = append(results, c.checkMSVCToolchainConfig())

	sdkResult, sdkBuild := c.checkWindowsSDK()
	results = append(results, sdkResult)

	if sdkBuild >= 26100 && c.EngineSourcePath != "" {
		results = append(results, c.checkNNERuntimeORTPatch())
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

	// Check required components
	requiredComponents := []struct {
		id   string
		name string
	}{
		{"Microsoft.VisualStudio.Workload.NativeDesktop", "Desktop development with C++"},
		{"Microsoft.VisualStudio.Workload.NativeGame", "Game development with C++"},
		{"Microsoft.VisualStudio.Component.VC.14.38.17.8.x86.x64", "MSVC v14.38"},
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

		args := []string{"modify", "--installPath", installs[0].InstallationPath}
		for id := range missingIDs {
			args = append(args, "--add", id)
		}
		args = append(args, "--passive")

		cmd := exec.Command(setupPath, args...)
		if err := cmd.Start(); err != nil {
			return CheckResult{
				Name:   "Visual Studio",
				Passed: false,
				Message: fmt.Sprintf("failed to launch VS Installer: %v", err),
			}
		}

		return CheckResult{
			Name:    "Visual Studio",
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("launched VS Installer to add: %s; re-run ludus init after installation completes",
				strings.Join(missing, ", ")),
		}
	}

	return CheckResult{
		Name:    "Visual Studio",
		Passed:  true,
		Message: fmt.Sprintf("%s with required workloads and MSVC v14.38", edition),
	}
}

const buildConfigXML = `<?xml version="1.0" encoding="utf-8" ?>
<Configuration xmlns="https://www.unrealengine.com/BuildConfiguration">
  <WindowsPlatform>
    <CompilerVersion>14.38.33130</CompilerVersion>
  </WindowsPlatform>
</Configuration>
`

func (c *Checker) checkMSVCToolchainConfig() CheckResult {
	configDir := filepath.Join(os.Getenv("APPDATA"), "Unreal Engine", "UnrealBuildTool")
	configPath := filepath.Join(configDir, "BuildConfiguration.xml")

	data, err := os.ReadFile(configPath)
	if err == nil && strings.Contains(string(data), "<CompilerVersion>14.38.33130</CompilerVersion>") {
		return CheckResult{
			Name:    "MSVC Toolchain Config",
			Passed:  true,
			Message: fmt.Sprintf("BuildConfiguration.xml pins MSVC 14.38 (%s)", configPath),
		}
	}

	if !c.Fix {
		hint := "file missing"
		if err == nil {
			hint = "CompilerVersion not set to 14.38.33130"
		}
		return CheckResult{
			Name:   "MSVC Toolchain Config",
			Passed: false,
			Message: fmt.Sprintf("%s at %s; run with --fix to create/update it",
				hint, configPath),
		}
	}

	// Auto-fix: write the config file
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return CheckResult{
			Name:    "MSVC Toolchain Config",
			Passed:  false,
			Message: fmt.Sprintf("failed to create directory %s: %v", configDir, err),
		}
	}

	if err := os.WriteFile(configPath, []byte(buildConfigXML), 0o644); err != nil {
		return CheckResult{
			Name:    "MSVC Toolchain Config",
			Passed:  false,
			Message: fmt.Sprintf("failed to write %s: %v", configPath, err),
		}
	}

	return CheckResult{
		Name:    "MSVC Toolchain Config",
		Passed:  true,
		Message: fmt.Sprintf("created %s (pins MSVC 14.38.33130)", configPath),
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

	// Auto-fix: insert PublicDefinitions.Add("INITGUID"); before PublicDependencyModuleNames
	marker := "PublicDependencyModuleNames"
	idx := strings.Index(content, marker)
	if idx == -1 {
		return CheckResult{
			Name:   "NNERuntimeORT Patch",
			Passed: false,
			Message: fmt.Sprintf("could not find %s in %s; patch manually per UE_SOURCE_PATCHES.md",
				marker, buildCSPath),
		}
	}

	// Find the start of the line containing the marker to preserve indentation
	lineStart := strings.LastIndex(content[:idx], "\n") + 1
	indent := content[lineStart:idx]
	if trimmed := strings.TrimLeft(indent, " \t"); len(trimmed) > 0 {
		indent = indent[:len(indent)-len(trimmed)]
	}

	patchLine := indent + "PublicDefinitions.Add(\"INITGUID\");\n"
	patched := content[:lineStart] + patchLine + content[lineStart:]

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
