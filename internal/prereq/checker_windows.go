//go:build windows

package prereq

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

	// Check for Smart App Control — blocks unsigned DLLs compiled from source.
	results = append(results, c.checkSmartAppControl())

	// Check for plugin DLL dependency issues. Certain UE versions build plugin
	// DLLs into their own Binaries/Win64/ subdirectory which is not in the DLL
	// search path when other plugins depend on them. This causes cook failures
	// with GetLastError=126 (ERROR_MOD_NOT_FOUND). The fix is version-specific
	// because Epic reorganizes plugin modules across versions.
	if c.EngineSourcePath != "" {
		results = append(results, c.checkPluginDLLDeps()...)
	}

	return results
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
	patchedContent := content[:lineEnd] + patchLine + content[lineEnd:]

	if err := os.WriteFile(buildCSPath, []byte(patchedContent), 0o644); err != nil {
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
	const requiredGB = 300

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
