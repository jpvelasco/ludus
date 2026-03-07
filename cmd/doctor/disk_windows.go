//go:build windows

package doctor

import (
	"syscall"
	"unsafe"
)

// getFreeDiskGB returns the free disk space in GB for the given path.
// Returns -1 if the space cannot be determined.
func getFreeDiskGB(path string) float64 {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytes uint64
	pathPtr, _ := syscall.UTF16PtrFromString(path)

	ret, _, _ := proc.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		0,
		0,
	)
	if ret == 0 {
		return -1
	}
	return float64(freeBytes) / (1024 * 1024 * 1024)
}
