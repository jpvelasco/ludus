//go:build !windows

package game

import (
	"fmt"
	"os"
)

// totalRAMGB returns total physical memory in GB, or 0 if detection fails.
func totalRAMGB() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	var memKB uint64
	if _, err := fmt.Fscanf(f, "MemTotal: %d kB", &memKB); err != nil || memKB == 0 {
		return 0
	}

	return int(memKB / (1024 * 1024))
}
