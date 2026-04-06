package dockerbuild

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
