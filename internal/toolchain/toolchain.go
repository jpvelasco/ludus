package toolchain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// BuildVersion matches the Engine/Build/Build.version JSON structure.
type BuildVersion struct {
	MajorVersion int `json:"MajorVersion"`
	MinorVersion int `json:"MinorVersion"`
	PatchVersion int `json:"PatchVersion"`
}

// ToolchainSpec describes the required toolchain for an engine version.
type ToolchainSpec struct {
	SDKVersion string // e.g. "v22"
	ClangMajor int    // e.g. 18
	DirPrefix  string // e.g. "v22_clang-18" — used for directory matching
}

// CheckResult holds the outcome of a toolchain check.
type CheckResult struct {
	EngineVersion string // e.g. "5.6"
	VersionSource string // "Build.version" or "config"
	Required      *ToolchainSpec
	Found         bool
	FoundPath     string
	Message       string
}

// toolchainMap maps engine major.minor versions to their required toolchain.
var toolchainMap = map[string]ToolchainSpec{
	"5.4": {SDKVersion: "v21", ClangMajor: 16, DirPrefix: "v21_clang-16"},
	"5.5": {SDKVersion: "v22", ClangMajor: 18, DirPrefix: "v22_clang-18"},
	"5.6": {SDKVersion: "v22", ClangMajor: 18, DirPrefix: "v22_clang-18"},
	"5.7": {SDKVersion: "v23", ClangMajor: 20, DirPrefix: "v23_clang-20"},
}

// ParseBuildVersion reads and parses the Build.version JSON file from the engine source.
func ParseBuildVersion(engineSourcePath string) (*BuildVersion, error) {
	path := filepath.Join(engineSourcePath, "Engine", "Build", "Build.version")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v BuildVersion
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing Build.version: %w", err)
	}
	return &v, nil
}

// DetectEngineVersion tries to determine the engine major.minor version.
// It first reads Build.version from the engine source. If that fails, it
// falls back to the config string (e.g. "5.6.1" -> "5.6").
// Returns the version string and its source ("Build.version" or "config"),
// or empty strings if neither is available.
func DetectEngineVersion(engineSourcePath, configVersion string) (version, source string) {
	if engineSourcePath != "" {
		if bv, err := ParseBuildVersion(engineSourcePath); err == nil {
			return fmt.Sprintf("%d.%d", bv.MajorVersion, bv.MinorVersion), "Build.version"
		}
	}

	if configVersion != "" {
		parts := strings.SplitN(configVersion, ".", 3)
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1], "config"
		}
	}

	return "", ""
}

// LookupToolchain returns the toolchain spec for a given engine major.minor
// version, or nil if the version has no known mapping.
func LookupToolchain(version string) *ToolchainSpec {
	if spec, ok := toolchainMap[version]; ok {
		return &spec
	}
	return nil
}

// CheckToolchain orchestrates engine version detection and platform-specific
// toolchain validation. It returns a CheckResult describing whether the
// required toolchain was found.
func CheckToolchain(engineSourcePath, configVersion string) CheckResult {
	if engineSourcePath == "" {
		return CheckResult{
			Message: "skipped (no engine source path)",
		}
	}

	version, source := DetectEngineVersion(engineSourcePath, configVersion)
	if version == "" {
		return CheckResult{
			Message: "could not detect engine version",
		}
	}

	spec := LookupToolchain(version)
	if spec == nil {
		return CheckResult{
			EngineVersion: version,
			VersionSource: source,
			Message:       fmt.Sprintf("engine %s has no known toolchain mapping", version),
		}
	}

	result := CheckResult{
		EngineVersion: version,
		VersionSource: source,
		Required:      spec,
	}

	if runtime.GOOS == "windows" {
		return checkToolchainWindows(engineSourcePath, result)
	}
	return checkToolchainLinux(engineSourcePath, result)
}

func checkToolchainLinux(engineSourcePath string, result CheckResult) CheckResult {
	spec := result.Required

	// Primary location: engine bundled SDK
	sdkDir := filepath.Join(engineSourcePath, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64")
	if found, path := findToolchainDir(sdkDir, spec.DirPrefix); found {
		result.Found = true
		result.FoundPath = path
		result.Message = fmt.Sprintf("toolchain %s found at %s (engine %s from %s)",
			spec.DirPrefix, path, result.EngineVersion, result.VersionSource)
		return result
	}

	// Fallback: LINUX_MULTIARCH_ROOT
	if multiarchRoot := os.Getenv("LINUX_MULTIARCH_ROOT"); multiarchRoot != "" {
		if found, path := findToolchainDir(multiarchRoot, spec.DirPrefix); found {
			result.Found = true
			result.FoundPath = path
			result.Message = fmt.Sprintf("toolchain %s found via LINUX_MULTIARCH_ROOT at %s (engine %s from %s)",
				spec.DirPrefix, path, result.EngineVersion, result.VersionSource)
			return result
		}
	}

	result.Message = fmt.Sprintf("toolchain %s not found for engine %s; run Setup.sh or see Epic docs",
		spec.DirPrefix, result.EngineVersion)
	return result
}

func checkToolchainWindows(_ string, result CheckResult) CheckResult {
	spec := result.Required

	multiarchRoot := os.Getenv("LINUX_MULTIARCH_ROOT")
	if multiarchRoot == "" {
		result.Message = fmt.Sprintf("LINUX_MULTIARCH_ROOT not set (needed for Linux cross-compile, requires %s)",
			spec.DirPrefix)
		return result
	}

	if found, path := findToolchainDir(multiarchRoot, spec.DirPrefix); found {
		result.Found = true
		result.FoundPath = path
		result.Message = fmt.Sprintf("toolchain %s found via LINUX_MULTIARCH_ROOT (engine %s from %s)",
			spec.DirPrefix, result.EngineVersion, result.VersionSource)
		return result
	}

	result.Message = fmt.Sprintf("toolchain %s not found in LINUX_MULTIARCH_ROOT (%s) for engine %s",
		spec.DirPrefix, multiarchRoot, result.EngineVersion)
	return result
}

// findToolchainDir scans parentDir for a directory entry whose name starts
// with prefix (e.g. "v22_clang-18" matches "v22_clang-18.1.8-centos7").
// Returns whether a match was found and the full path.
func findToolchainDir(parentDir, prefix string) (bool, string) {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return false, ""
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			return true, filepath.Join(parentDir, e.Name())
		}
	}
	return false, ""
}
