//go:build windows

package dockerbuild

import (
	"syscall"
	"unsafe"
)

type dockerbuildMemoryStatusEx struct {
	length               uint32
	memoryLoad           uint32
	totalPhys            uint64
	availPhys            uint64
	totalPageFile        uint64
	availPageFile        uint64
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

// totalRAMGB returns total physical memory in GB, or 0 if detection fails.
func totalRAMGB() int {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GlobalMemoryStatusEx")

	var mem dockerbuildMemoryStatusEx
	mem.length = uint32(unsafe.Sizeof(mem))

	ret, _, _ := proc.Call(uintptr(unsafe.Pointer(&mem)))
	if ret == 0 {
		return 0
	}

	return int(mem.totalPhys / (1024 * 1024 * 1024))
}
