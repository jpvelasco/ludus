//go:build darwin

package dockerbuild

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// totalRAMGB returns total physical memory in GB, or 0 if detection fails.
// macOS has no /proc/meminfo, so read total physical memory via sysctl.
func totalRAMGB() int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0
	}
	bytes, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil || bytes == 0 {
		return 0
	}
	return int(bytes / (1024 * 1024 * 1024))
}
