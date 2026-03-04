//go:build windows

package game

import (
	"os/exec"
	"strings"
)

// readSystemEnvVar reads a system-level environment variable from the Windows
// registry. This is useful when the current process was started before the
// variable was set (e.g. after a toolchain installer sets LINUX_MULTIARCH_ROOT
// without the user restarting their terminal).
func readSystemEnvVar(name string) string {
	out, err := exec.Command("reg", "query",
		`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
		"/v", name,
	).Output()
	if err != nil {
		return ""
	}
	// Output format: "    LINUX_MULTIARCH_ROOT    REG_SZ    C:\UnrealToolchains\..."
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, name) {
			parts := strings.SplitN(line, "REG_SZ", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
			parts = strings.SplitN(line, "REG_EXPAND_SZ", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}
