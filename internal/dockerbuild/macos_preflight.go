package dockerbuild

import (
	"context"
	"fmt"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/toolchain"
)

// MacOSPreflightOptions configures macOS-specific pre-flight container runs.
type MacOSPreflightOptions struct {
	EngineSourcePath string
	EngineVersion    string
	BaseImage        string
	Runtime          string // "docker" or "podman"
	Arch             string // "arm64" or "amd64"
}

func (o MacOSPreflightOptions) platformString() string {
	arch := o.Arch
	if arch == "" {
		arch = "amd64"
	}
	return "linux/" + arch
}

func (o MacOSPreflightOptions) baseImage() string {
	if o.BaseImage != "" {
		return o.BaseImage
	}
	return "ubuntu:22.04"
}

// LinuxToolchainPresent returns true if the Linux cross-compile toolchain for
// the given engine version is already present in the engine source tree.
func LinuxToolchainPresent(engineSourcePath, version string) bool {
	_, found := toolchain.LinuxToolchainPath(engineSourcePath, version)
	return found
}

// RunLinuxToolchainBootstrap runs Setup.sh inside a throwaway Linux container
// mounted to the host engine tree, causing Epic's downloader to fetch the Linux
// cross-compile toolchain into the host filesystem. Skips if already present.
func RunLinuxToolchainBootstrap(ctx context.Context, opts MacOSPreflightOptions, r *runner.Runner) error {
	if LinuxToolchainPresent(opts.EngineSourcePath, opts.EngineVersion) {
		return nil // already present — skip
	}

	cli := ContainerCLI(opts.Runtime)
	fmt.Println("  Fetching Linux toolchain (one-time, ~2 GB)...")
	return r.Run(ctx,
		cli, "run", "--rm",
		"--platform", opts.platformString(),
		"-v", opts.EngineSourcePath+":/engine",
		"-w", "/engine",
		opts.baseImage(),
		"bash", "Setup.sh",
	)
}

// RunLinuxGenerateProjectFiles runs GenerateProjectFiles.sh -makefile inside a
// throwaway Linux container mounted to the host engine tree, producing a
// Linux-targeted Makefile with explicit Linux build targets.
func RunLinuxGenerateProjectFiles(ctx context.Context, opts MacOSPreflightOptions, r *runner.Runner) error {
	cli := ContainerCLI(opts.Runtime)
	fmt.Println("  Generating Linux project files...")
	return r.Run(ctx,
		cli, "run", "--rm",
		"--platform", opts.platformString(),
		"-v", opts.EngineSourcePath+":/engine",
		"-w", "/engine",
		opts.baseImage(),
		"bash", "GenerateProjectFiles.sh", "-makefile",
	)
}
