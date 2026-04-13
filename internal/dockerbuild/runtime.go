package dockerbuild

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Backend constants. Use these instead of raw string comparisons.
const (
	BackendDocker = "docker"
	BackendPodman = "podman"
	BackendNative = "native"
	BackendWSL2   = "wsl2"
)

// IsContainerBackend returns true if the backend is a container runtime
// (i.e. docker or podman) rather than a native build.
func IsContainerBackend(backend string) bool {
	return backend == BackendDocker || backend == BackendPodman
}

// IsWSL2Backend returns true if the backend is the WSL2 native build backend.
func IsWSL2Backend(backend string) bool {
	return backend == BackendWSL2
}

// ContainerCLI returns the path to the container CLI binary for the given
// backend. It first tries the system PATH, then falls back to known install
// locations on Windows (winget installs Podman to Program Files but the
// current shell may not have reloaded PATH yet).
//
// If the binary is not found anywhere, it returns the bare name (e.g. "docker")
// so that runner.Run produces a clear "command not found" error. The prereq
// checker validates availability before builds start.
func ContainerCLI(backend string) string {
	name := BackendDocker
	if backend == BackendPodman {
		name = BackendPodman
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	if backend == BackendPodman {
		if p := ResolvePodmanFallback(); p != "" {
			return p
		}
	}
	return name
}

// podmanWindowsPaths are the default install locations checked when podman
// is not in the current PATH (e.g. terminal not restarted after install).
var podmanWindowsPaths = []string{
	`C:\Program Files\RedHat\Podman\podman.exe`,
}

// ResolvePodmanFallback checks default install locations on Windows.
// Returns the full path if found, empty string otherwise.
func ResolvePodmanFallback() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	for _, p := range podmanWindowsPaths {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	return ""
}

// wrapBuildError adds actionable guidance to container build failures.
// Detects Docker Desktop's containerd lease timeout (which crashes on large
// UE5 image exports) and recommends Podman as an alternative.
func wrapBuildError(cli string, err error) error {
	msg := err.Error()
	if cli == BackendDocker && isContainerdLeaseError(msg) {
		return fmt.Errorf("%s build failed (containerd lease timeout during image export): %w\n\n"+
			"Docker Desktop's containerd storage backend has a lease timeout that crashes\n"+
			"during export of large images (UE5 engine images are 60-100+ GB).\n\n"+
			"Recommended fix: use Podman instead:\n"+
			"  podman machine init && podman machine start\n"+
			"  ludus engine build --backend podman\n\n"+
			"Or use --skip-engine to package pre-built binaries (much smaller image):\n"+
			"  ludus engine build --backend podman --skip-engine", cli, err)
	}
	return fmt.Errorf("%s build failed: %w", cli, err)
}

// isContainerdLeaseError checks if an error message indicates Docker Desktop's
// containerd lease timeout failure during image export. Matches two known error
// patterns: (1) "lease ... not found" and (2) "failed to solve ... exporting to image".
func isContainerdLeaseError(msg string) bool {
	return (strings.Contains(msg, "lease") && strings.Contains(msg, "not found")) ||
		(strings.Contains(msg, "failed to solve") && strings.Contains(msg, "exporting to image"))
}
