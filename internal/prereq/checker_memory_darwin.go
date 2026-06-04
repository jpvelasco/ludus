//go:build darwin

package prereq

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func (c *Checker) checkMemory() CheckResult {
	totalGB, err := readMemTotalGBDarwin()
	if err != nil {
		return CheckResult{Name: "Memory", Passed: false, Message: err.Error()}
	}
	return memoryResult(totalGB)
}

// readMemTotalGBDarwin reads total physical memory via sysctl hw.memsize.
func readMemTotalGBDarwin() (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize")
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			stderr = ": " + strings.TrimSpace(string(ee.Stderr))
		}
		return 0, fmt.Errorf("cannot read memory via sysctl: %w%s", err, stderr)
	}
	bytes, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse sysctl hw.memsize output: %w", err)
	}
	return bytes / (1024 * 1024 * 1024), nil
}
