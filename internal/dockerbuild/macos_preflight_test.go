package dockerbuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

func TestLinuxToolchainPresent_Found(t *testing.T) {
	root := t.TempDir()
	sdkDir := filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64", "v26_clang-20.1.8-rockylinux8")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if !LinuxToolchainPresent(root, "5.7") {
		t.Error("expected toolchain to be found")
	}
}

func TestLinuxToolchainPresent_Missing(t *testing.T) {
	root := t.TempDir()
	if LinuxToolchainPresent(root, "5.7") {
		t.Error("expected toolchain to be absent in empty dir")
	}
}

func TestLinuxToolchainPresent_UnknownVersion(t *testing.T) {
	root := t.TempDir()
	if LinuxToolchainPresent(root, "4.99") {
		t.Error("expected false for unknown engine version")
	}
}

func TestMacOSPreflightOptions_PlatformString(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"arm64", "linux/arm64"},
		{"amd64", "linux/amd64"},
		{"", "linux/amd64"},
	}
	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			opts := MacOSPreflightOptions{Arch: tt.arch}
			got := opts.platformString()
			if got != tt.want {
				t.Errorf("platformString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunLinuxToolchainBootstrap_DryRun(t *testing.T) {
	root := t.TempDir()
	r := runner.NewRunner(false, true) // dry-run — command printed, not executed
	opts := MacOSPreflightOptions{
		EngineSourcePath: root,
		EngineVersion:    "5.7",
		BaseImage:        "ubuntu:22.04",
		Runtime:          "docker",
		Arch:             "arm64",
	}
	if err := RunLinuxToolchainBootstrap(context.Background(), opts, r); err != nil {
		t.Errorf("unexpected error in dry-run: %v", err)
	}
}

func TestRunLinuxToolchainBootstrap_SkipsWhenPresent(t *testing.T) {
	root := t.TempDir()
	sdkDir := filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64", "v26_clang-20.1.8-rockylinux8")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Use a non-dry-run runner — if the command were invoked it would fail
	// (docker not available in CI). Toolchain present means skip, so no error.
	r := runner.NewRunner(false, false)
	opts := MacOSPreflightOptions{
		EngineSourcePath: root,
		EngineVersion:    "5.7",
		BaseImage:        "ubuntu:22.04",
		Runtime:          "docker",
		Arch:             "arm64",
	}
	if err := RunLinuxToolchainBootstrap(context.Background(), opts, r); err != nil {
		t.Errorf("unexpected error when toolchain already present: %v", err)
	}
}

func TestRunLinuxGenerateProjectFiles_DryRun(t *testing.T) {
	root := t.TempDir()
	r := runner.NewRunner(false, true)
	opts := MacOSPreflightOptions{
		EngineSourcePath: root,
		EngineVersion:    "5.7",
		BaseImage:        "ubuntu:22.04",
		Runtime:          "docker",
		Arch:             "arm64",
	}
	if err := RunLinuxGenerateProjectFiles(context.Background(), opts, r); err != nil {
		t.Errorf("unexpected error in dry-run: %v", err)
	}
}

func TestPreflightInstallCmd_ContainsBuildDeps(t *testing.T) {
	cmd := preflightInstallCmd("bash Setup.sh")
	if !strings.Contains(cmd, "apt-get install") {
		t.Error("expected apt-get install in preflight command")
	}
	if !strings.Contains(cmd, "bash Setup.sh") {
		t.Error("expected script invocation in preflight command")
	}
	for _, pkg := range []string{"build-essential", "git", "cmake", "python3"} {
		if !strings.Contains(cmd, pkg) {
			t.Errorf("expected %q in preflight install command", pkg)
		}
	}
}

func TestPreflightInstallCmd_GenerateProjectFiles(t *testing.T) {
	cmd := preflightInstallCmd("bash GenerateProjectFiles.sh -makefile")
	if !strings.Contains(cmd, "bash GenerateProjectFiles.sh -makefile") {
		t.Error("expected GenerateProjectFiles.sh invocation in command")
	}
}
