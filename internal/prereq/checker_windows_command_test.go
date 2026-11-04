//go:build windows

package prereq

import (
	"strings"
	"testing"
)

func TestToolchainPowerShellCommandPassesInstallerPathThroughEnvironment(t *testing.T) {
	installerPath := `C:\Users\a'b\Program Files\toolchain.exe`
	cmd := toolchainPowerShellCommand(installerPath,
		"Start-Process -FilePath $env:LUDUS_TOOLCHAIN_INSTALLER_PATH -Verb RunAs -Wait")
	if strings.Contains(cmd.Args[len(cmd.Args)-1], installerPath) {
		t.Fatalf("PowerShell script interpolates installer path: %s", cmd.Args[len(cmd.Args)-1])
	}
	for _, entry := range cmd.Env {
		if entry == "LUDUS_TOOLCHAIN_INSTALLER_PATH="+installerPath {
			return
		}
	}
	t.Fatalf("command environment does not preserve installer path %q", installerPath)
}
