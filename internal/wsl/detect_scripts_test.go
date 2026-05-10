package wsl

import (
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/dockerbuild"
)

func TestInstallDepsScriptIncludesRuntimePackages(t *testing.T) {
	script := installDepsScript()

	// Every runtime package from AptRuntimePackages must appear in the script.
	// This is the single source of truth for UE5 runtime deps (identified via
	// ldd audit of UnrealEditor-Cmd).
	for _, pkg := range dockerbuild.AptRuntimePackages {
		if !strings.Contains(script, pkg) {
			t.Errorf("installDepsScript() missing runtime package %q", pkg)
		}
	}

	// Build deps must still be present.
	for _, pkg := range []string{"build-essential", "cmake", "python3", "rsync"} {
		if !strings.Contains(script, pkg) {
			t.Errorf("installDepsScript() missing build package %q", pkg)
		}
	}
}

func TestInstallRuntimeDepsScript(t *testing.T) {
	script := installRuntimeDepsScript()

	for _, pkg := range dockerbuild.AptRuntimePackages {
		if !strings.Contains(script, pkg) {
			t.Errorf("installRuntimeDepsScript() missing package %q", pkg)
		}
	}

	// Must be non-interactive.
	if !strings.Contains(script, "DEBIAN_FRONTEND=noninteractive") {
		t.Error("runtime deps script must set DEBIAN_FRONTEND=noninteractive")
	}
	if !strings.Contains(script, "-y") {
		t.Error("runtime deps script must use -y for non-interactive install")
	}
}

func TestInstallDepsScriptIsIdempotent(t *testing.T) {
	script := installDepsScript()

	// apt-get install -y is idempotent: re-installing already-installed
	// packages is a no-op. Verify the script uses -y.
	if !strings.Contains(script, "install -y") {
		t.Error("installDepsScript() must use 'install -y' for idempotent operation")
	}
}

func TestInstallDepsScriptHandlesT64Packages(t *testing.T) {
	script := installDepsScript()

	// The script must handle Ubuntu 24.04 t64 package renames.
	for oldName, t64Name := range dockerbuild.AptRuntimeT64Packages {
		if !strings.Contains(script, oldName) && !strings.Contains(script, t64Name) {
			t.Errorf("script missing both %q and t64 variant %q", oldName, t64Name)
		}
		if !strings.Contains(script, t64Name) {
			t.Errorf("script missing t64 fallback %q for %q", t64Name, oldName)
		}
	}
}

func TestInstallScriptsRunAsRoot(t *testing.T) {
	// Scripts run via wsl.exe -u root, so they must NOT use sudo.
	// Using sudo under -u root is redundant and may fail if sudo is not installed.
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"installDepsScript", installDepsScript()},
		{"installRuntimeDepsScript", installRuntimeDepsScript()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Contains(tc.script, "sudo ") {
				t.Errorf("%s should not use sudo (runs as root via wsl.exe -u root)", tc.name)
			}
		})
	}
}
