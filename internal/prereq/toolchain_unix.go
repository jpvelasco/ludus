//go:build !windows

package prereq

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/jpvelasco/ludus/internal/toolchain"
)

// installUnixCrossCompileToolchain downloads the Epic CrossToolchain_Linux
// installer and extracts it into Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/Linux_x64/<version>.
// The published .exe is an NSIS wrapper around a 7z archive, so 7z is enough.
func installUnixCrossCompileToolchain(engineSourcePath string, tc toolchain.CheckResult) CheckResult {
	spec := tc.Required
	if spec == nil || spec.InstallerURL == "" {
		return CheckResult{Name: "Toolchain", Passed: true, Warning: true, Message: "no installer URL available for this engine version"}
	}
	if engineSourcePath == "" {
		return toolchainFailure("cannot install toolchain: engine source path is not set")
	}
	if dirName := toolchainDirName(spec); !strings.HasPrefix(dirName, spec.DirPrefix) {
		return toolchainFailure("installer %s does not match expected toolchain %s; extract it manually into Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/Linux_x64/<version>/", path.Base(spec.InstallerURL), spec.DirPrefix)
	}
	sevenZ, err := exec.LookPath("7z")
	if err != nil {
		return toolchainFailure("7z not found in PATH; install p7zip-full (or equivalent) to extract %s, or extract the installer manually into Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/Linux_x64/%s/", spec.DirPrefix, toolchainDirName(spec))
	}
	installerPath, err := ensureToolchainInstaller(spec)
	if err != nil {
		return toolchainFailure("failed to download toolchain installer: %v", err)
	}
	return extractUnixToolchain(engineSourcePath, spec, sevenZ, installerPath)
}

// toolchainArchDir is the host-architecture directory every Epic Linux
// toolchain archive contains; its presence proves an extraction is usable.
const toolchainArchDir = "x86_64-unknown-linux-gnu"

func toolchainFailure(format string, args ...any) CheckResult {
	return CheckResult{Name: "Toolchain", Passed: false, Message: fmt.Sprintf(format, args...)}
}

// toolchainDirName is the versioned directory UE expects under Linux_x64
// (e.g. v26_clang-20.1.8-rockylinux8), taken from the installer file name.
func toolchainDirName(spec *toolchain.ToolchainSpec) string {
	return strings.TrimSuffix(path.Base(spec.InstallerURL), ".exe")
}

func cachedInstallerPath(spec *toolchain.ToolchainSpec) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("ludus-toolchain-%s.exe", spec.SDKVersion))
}

func ensureToolchainInstaller(spec *toolchain.ToolchainSpec) (string, error) {
	installerPath := cachedInstallerPath(spec)
	if _, err := os.Stat(installerPath); err == nil {
		fmt.Printf("Using cached toolchain installer: %s\n", installerPath)
		return installerPath, nil
	}
	fmt.Printf("Downloading cross-compile toolchain (%s)...\n", spec.DirPrefix)
	fmt.Printf("  URL: %s\n", spec.InstallerURL)
	fmt.Println("  This is a large download (400-1000 MB), please be patient.")
	if err := downloadFile(installerPath, spec.InstallerURL); err != nil {
		return "", err
	}
	fmt.Printf("  Downloaded to %s\n", installerPath)
	return installerPath, nil
}

// extractUnixToolchain extracts the installer into a staging directory and
// renames it to Linux_x64/<version>/, so a failed or unexpected extraction
// never leaves a partial tree in Linux_x64. The installer is removed after a
// successful install and kept otherwise so a retry can reuse it.
func extractUnixToolchain(engineSourcePath string, spec *toolchain.ToolchainSpec, sevenZ, installerPath string) CheckResult {
	sdkDir := filepath.Join(engineSourcePath, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64")
	if err := os.MkdirAll(sdkDir, 0755); err != nil {
		return toolchainFailure("failed to create toolchain directory: %v", err)
	}
	staging, err := os.MkdirTemp(sdkDir, ".ludus-extract-")
	if err != nil {
		return toolchainFailure("failed to create toolchain staging directory: %v", err)
	}
	defer os.RemoveAll(staging)

	target := filepath.Join(sdkDir, toolchainDirName(spec))
	fmt.Printf("Extracting toolchain into %s...\n", target)
	out, err := exec.Command(sevenZ, "x", "-y", "-o"+staging, installerPath).CombinedOutput()
	if err != nil {
		return toolchainFailure("failed to extract toolchain installer with 7z: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	if err := promoteExtractedToolchain(staging, target, spec.DirPrefix); err != nil {
		return toolchainFailure("%v; delete %s and re-run 'ludus init --fix' to download it again", err, installerPath)
	}
	if err := os.Remove(installerPath); err == nil {
		fmt.Printf("  Removed installer %s\n", installerPath)
	}
	return CheckResult{Name: "Toolchain", Passed: true, Message: fmt.Sprintf("toolchain %s extracted to %s", spec.DirPrefix, target)}
}

// promoteExtractedToolchain moves the extracted toolchain root to target.
// Epic's archives are flat, but a top-level version folder is also accepted.
func promoteExtractedToolchain(staging, target, dirPrefix string) error {
	root := staging
	if nested := findExtractedToolchain(staging, dirPrefix); nested != "" {
		root = nested
	}
	if _, err := os.Stat(filepath.Join(root, toolchainArchDir)); err != nil {
		return fmt.Errorf("extracted installer does not contain %s/", toolchainArchDir)
	}
	if err := os.Rename(root, target); err != nil {
		return fmt.Errorf("failed to move extracted toolchain into place: %w", err)
	}
	return nil
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
