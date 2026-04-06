package dockerbuild

import (
	"fmt"
	"strings"
)

// IsContainerBackend returns true if the backend is a container runtime
// (i.e. "docker" or "podman") rather than a native build.
func IsContainerBackend(backend string) bool {
	return backend == "docker" || backend == "podman"
}

// ContainerCLI returns the CLI binary name for the given backend.
// Defaults to "docker" for unrecognized values.
func ContainerCLI(backend string) string {
	if backend == "podman" {
		return "podman"
	}
	return "docker"
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
// containerd lease timeout failure during image export.
func isContainerdLeaseError(msg string) bool {
	return strings.Contains(msg, "lease") && strings.Contains(msg, "not found") ||
		strings.Contains(msg, "failed to solve") && strings.Contains(msg, "exporting to image")
}
