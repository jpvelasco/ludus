//go:build !windows

package doctor

import "syscall"

// getFreeDiskGB returns the free disk space in GB for the given path.
// Returns -1 if the space cannot be determined.
func getFreeDiskGB(path string) float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return -1
	}
	return float64(stat.Bavail*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
}
