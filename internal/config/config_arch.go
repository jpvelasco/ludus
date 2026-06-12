package config

import "strings"

// NormalizeArch maps architecture aliases to Go's GOARCH values.
// Accepts: amd64, x86_64, arm64, aarch64 (case-insensitive). Defaults to "amd64".
func NormalizeArch(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "arm64", "aarch64":
		return "arm64"
	case "amd64", "x86_64", "":
		return "amd64"
	default:
		return "amd64"
	}
}

// ServerPlatformDir returns the UE output directory name for server builds.
// amd64 → "LinuxServer", arm64 → "LinuxArm64Server".
func ServerPlatformDir(arch string) string {
	if NormalizeArch(arch) == "arm64" {
		return "LinuxArm64Server"
	}
	return "LinuxServer"
}

// BinariesPlatformDir returns the UE Binaries subdirectory for the architecture.
// amd64 → "Linux", arm64 → "LinuxArm64".
func BinariesPlatformDir(arch string) string {
	if NormalizeArch(arch) == "arm64" {
		return "LinuxArm64"
	}
	return "Linux"
}

// UEPlatformName returns the UE platform name used in RunUAT -platform= flag.
// amd64 → "Linux", arm64 → "Linux" (arm64 targeting is done via TargetArchitecture INI).
func UEPlatformName(arch string) string {
	return "Linux"
}
