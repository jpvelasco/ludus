package toolchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func toolchain58Spec(t *testing.T) *ToolchainSpec {
	t.Helper()
	spec := LookupToolchain("5.8")
	if spec == nil {
		t.Fatal("no toolchain spec for 5.8")
	}
	return spec
}

func TestCheckToolchainLinux_FoundInEngineSDK(t *testing.T) {
	spec := toolchain58Spec(t)
	engineRoot := t.TempDir()
	sdkDir := filepath.Join(engineRoot, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64")
	toolDir := filepath.Join(sdkDir, spec.DirPrefix+"-20.1.8-rockylinux8")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}

	result := checkToolchainLinux(engineRoot, CheckResult{
		EngineVersion: "5.8",
		VersionSource: "config",
		Required:      spec,
	})
	if !result.Found {
		t.Errorf("checkToolchainLinux() Found = false, want true: %+v", result)
	}
	if !strings.Contains(result.Message, "found at") {
		t.Errorf("message %q missing 'found at'", result.Message)
	}
}

func TestCheckToolchainLinux_FoundViaMultiarchRoot(t *testing.T) {
	spec := toolchain58Spec(t)
	root := t.TempDir()
	toolDir := filepath.Join(root, spec.DirPrefix+"-20.1.8-rockylinux8")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINUX_MULTIARCH_ROOT", root)

	result := checkToolchainLinux(t.TempDir(), CheckResult{
		EngineVersion: "5.8",
		VersionSource: "config",
		Required:      spec,
	})
	if !result.Found {
		t.Errorf("checkToolchainLinux() Found = false, want true via LINUX_MULTIARCH_ROOT: %+v", result)
	}
	if !strings.Contains(result.Message, "LINUX_MULTIARCH_ROOT") {
		t.Errorf("message %q missing LINUX_MULTIARCH_ROOT mention", result.Message)
	}
}

func TestCheckToolchainLinux_NotFound(t *testing.T) {
	spec := toolchain58Spec(t)
	t.Setenv("LINUX_MULTIARCH_ROOT", "")
	result := checkToolchainLinux(t.TempDir(), CheckResult{
		EngineVersion: "5.8",
		VersionSource: "config",
		Required:      spec,
	})
	if result.Found {
		t.Errorf("checkToolchainLinux() Found = true, want false: %+v", result)
	}
	if !strings.Contains(result.Message, "not found") {
		t.Errorf("message %q missing 'not found'", result.Message)
	}
}
