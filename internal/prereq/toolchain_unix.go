//go:build !windows

package prereq

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jpvelasco/ludus/internal/toolchain"
)

// installUnixCrossCompileToolchain downloads the Epic CrossToolchain_Linux
// installer and extracts it into Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/Linux_x64.
// The published .exe is an NSIS wrapper around a 7z archive, so 7z is enough.
func installUnixCrossCompileToolchain(engineSourcePath string, tc toolchain.CheckResult) CheckResult {
	spec := tc.Required
	if spec == nil || spec.InstallerURL == "" {
		return CheckResult{Name: "Toolchain", Passed: true, Warning: true, Message: "no installer URL available for this engine version"}
	}
	if engineSourcePath == "" {
		return CheckResult{Name: "Toolchain", Passed: false, Message: "cannot install toolchain: engine source path is not set"}
	}
	sevenZ, err := exec.LookPath("7z")
	if err != nil {
		return CheckResult{Name: "Toolchain", Passed: false, Message: fmt.Sprintf("7z not found in PATH; install p7zip-full (or equivalent) to extract %s, or extract the installer manually into Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/Linux_x64/", spec.DirPrefix)}
	}
	installerPath, err := ensureToolchainInstaller(spec)
	if err != nil {
		return CheckResult{Name: "Toolchain", Passed: false, Message: fmt.Sprintf("failed to download toolchain installer: %v", err)}
	}
	return extractUnixToolchain(engineSourcePath, spec, sevenZ, installerPath)
}

func ensureToolchainInstaller(spec *toolchain.ToolchainSpec) (string, error) {
	installerPath := filepath.Join(os.TempDir(), fmt.Sprintf("ludus-toolchain-%s.exe", spec.SDKVersion))
	if _, err := os.Stat(installerPath); err == nil {
		fmt.Printf("Using cached toolchain installer: %s\n", installerPath)
		return installerPath, nil
	}
	fmt.Printf("Downloading cross-compile toolchain (%s)...\n", spec.DirPrefix)
	fmt.Printf("  URL: %s\n", spec.InstallerURL)
	fmt.Println("  This is a large download (400-600 MB), please be patient.")
	if err := downloadFile(installerPath, spec.InstallerURL); err != nil {
		return "", err
	}
	fmt.Printf("  Downloaded to %s\n", installerPath)
	return installerPath, nil
}

func extractUnixToolchain(engineSourcePath string, spec *toolchain.ToolchainSpec, sevenZ, installerPath string) CheckResult {
	sdkDir := filepath.Join(engineSourcePath, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64")
	if err := os.MkdirAll(sdkDir, 0755); err != nil {
		return CheckResult{Name: "Toolchain", Passed: false, Message: fmt.Sprintf("failed to create toolchain directory: %v", err)}
	}
	fmt.Printf("Extracting toolchain into %s...\n", sdkDir)
	out, err := exec.Command(sevenZ, "x", "-y", "-o"+sdkDir, installerPath).CombinedOutput()
	if err != nil {
		return CheckResult{Name: "Toolchain", Passed: false, Message: fmt.Sprintf("failed to extract toolchain installer with 7z: %v\n%s", err, strings.TrimSpace(string(out)))}
	}
	if installed := findExtractedToolchain(sdkDir, spec.DirPrefix); installed != "" {
		return CheckResult{Name: "Toolchain", Passed: true, Message: fmt.Sprintf("toolchain %s extracted to %s", spec.DirPrefix, installed)}
	}
	return CheckResult{Name: "Toolchain", Passed: false, Message: fmt.Sprintf("extracted %s but toolchain directory %s was not found under %s", filepath.Base(installerPath), spec.DirPrefix, sdkDir)}
}

func findExtractedToolchain(sdkDir, dirPrefix string) string {
	entries, err := os.ReadDir(sdkDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), dirPrefix) {
			return filepath.Join(sdkDir, e.Name())
		}
	}
	return ""
}
