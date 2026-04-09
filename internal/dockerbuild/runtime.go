package dockerbuild

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// IsContainerBackend returns true if the backend is a container runtime
// (i.e. "docker" or "podman") rather than a native build.
func IsContainerBackend(backend string) bool {
	return backend == "docker" || backend == "podman"
}

// ContainerCLI returns the path to the container CLI binary for the given
// backend. It first tries the system PATH, then falls back to known install
// locations on Windows (winget installs Podman to Program Files but the
// current shell may not have reloaded PATH yet).
func ContainerCLI(backend string) string {
	name := "docker"
	if backend == "podman" {
		name = "podman"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	if backend == "podman" {
		if p := resolvePodmanFallback(); p != "" {
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

// resolvePodmanFallback checks default install locations on Windows.
// Returns the full path if found, empty string otherwise.
func resolvePodmanFallback() string {
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
	if cli == "docker" && isContainerdLeaseError(msg) {
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
