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

// fixCrossCompileToolchain is a no-op on non-Windows platforms.
// The cross-compile toolchain is only relevant for Windows → Linux cross-compilation.
func (c *Checker) fixCrossCompileToolchain(_ toolchain.CheckResult) CheckResult {
	return CheckResult{
		Name:    "Toolchain",
		Passed:  true,
		Warning: true,
		Message: "cross-compile toolchain install not supported on this platform",
	}
}
