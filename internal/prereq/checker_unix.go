//go:build !windows

package prereq

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/devrecon/ludus/internal/toolchain"
)

func (c *Checker) checkDiskSpace() CheckResult {
	checkPath := c.EngineSourcePath
	if checkPath == "" {
		checkPath = "."
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(checkPath, &stat); err != nil {
		return CheckResult{
			Name:    "Disk Space",
			Passed:  false,
			Message: fmt.Sprintf("failed to check disk space: %v", err),
		}
	}

	freeGB := (stat.Bavail * uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	const requiredGB = 100

	if freeGB < requiredGB {
		return CheckResult{
			Name:    "Disk Space",
			Passed:  false,
			Message: fmt.Sprintf("%d GB free, need %d GB", freeGB, requiredGB),
		}
	}

	return CheckResult{
		Name:    "Disk Space",
		Passed:  true,
		Message: fmt.Sprintf("%d GB free", freeGB),
	}
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

func (c *Checker) checkMemory() CheckResult {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return CheckResult{
			Name:    "Memory",
			Passed:  false,
			Message: fmt.Sprintf("cannot read /proc/meminfo: %v", err),
		}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				break
			}
			kB, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				break
			}
			totalGB := kB / (1024 * 1024)
			const requiredGB = 16
			if totalGB < requiredGB {
				return CheckResult{
					Name:    "Memory",
					Passed:  false,
					Message: fmt.Sprintf("%d GB total, need %d GB", totalGB, requiredGB),
				}
			}
			return CheckResult{
				Name:    "Memory",
				Passed:  true,
				Message: fmt.Sprintf("%d GB total", totalGB),
			}
		}
	}

	return CheckResult{
		Name:    "Memory",
		Passed:  false,
		Message: "could not parse /proc/meminfo",
	}
}
