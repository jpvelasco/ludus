//go:build !windows && !darwin

package prereq

import (
	"fmt"
	"syscall"

	"github.com/jpvelasco/ludus/internal/toolchain"
)

func (c *Checker) checkDiskSpace() CheckResult {
	checkPath := c.diskCheckPath()

	var stat syscall.Statfs_t
	if err := syscall.Statfs(checkPath, &stat); err != nil {
		return CheckResult{
			Name:    "Disk Space",
			Passed:  false,
			Message: fmt.Sprintf("failed to check disk space: %v", err),
		}
	}

	freeGB := (stat.Bavail * uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	return diskSpaceResult(freeGB, c.Backend)
}

func (c *Checker) platformChecks() []CheckResult {
	return nil
}

// fixCrossCompileToolchain downloads Epic's NSIS toolchain installer and
// extracts it with 7z into the engine HostLinux SDK tree. The .exe is a
// 7z-readable archive; no NSIS execution is needed on Unix.
func (c *Checker) fixCrossCompileToolchain(tc toolchain.CheckResult) CheckResult {
	return installUnixCrossCompileToolchain(c.EngineSourcePath, tc)
}
