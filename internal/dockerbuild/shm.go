package dockerbuild

// DefaultShmSize is the /dev/shm size requested for build containers.
// Docker's 64 MB default forces UnrealBuildAccelerator to fall back to slower
// disk-backed file mapping; it needs at least 1 GB.
const DefaultShmSize = "2g"

func shmSizeArgs(size string) []string {
	if size == "" {
		size = DefaultShmSize
	}
	return []string{"--shm-size", size}
}
