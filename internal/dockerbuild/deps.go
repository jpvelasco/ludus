package dockerbuild

import (
	"fmt"
	"strings"
)

// UE5 container image dependencies, organized by purpose.
//
// Build-time packages are needed to compile the engine and game (compilers, linkers, etc.).
// Runtime packages are needed by UnrealEditor-Cmd during the cook step. UE5 links against
// X11, NSS, accessibility, and audio libraries even in headless/server mode.
//
// The runtime list was identified by running `ldd /engine/Engine/Binaries/Linux/UnrealEditor-Cmd`
// inside the container image and collecting every "not found" entry. All four critical binaries
// (UnrealEditor-Cmd, ShaderCompileWorker, UnrealPak, CrashReportClient) were audited.

// aptBuildPackages are the Debian/Ubuntu packages required to compile UE5 from source.
var aptBuildPackages = []string{
	"build-essential",
	"git",
	"cmake",
	"python3",
	"curl",
	"xdg-user-dirs",
	"shared-mime-info",
	"libfontconfig1",
	"libfreetype6",
	"libc6-dev",
}

// AptRuntimePackages are the Debian/Ubuntu packages required at runtime by
// UnrealEditor-Cmd. Without these, the cook step fails with:
//
//	error while loading shared libraries: <lib>.so: cannot open shared object file
//
// Exported so tests can verify both the Dockerfile and game build preamble
// stay in sync with this list.
var AptRuntimePackages = []string{
	"libnss3",            // NSS (TLS/crypto) — also provides libnssutil3, libsmime3
	"libnspr4",           // Netscape Portable Runtime (NSS dependency)
	"libdbus-1-3",        // D-Bus IPC
	"libatk1.0-0",        // Accessibility toolkit
	"libatk-bridge2.0-0", // AT-SPI2 ATK bridge (also pulls in libatspi2.0-0)
	"libdrm2",            // Direct Rendering Manager
	"libxcomposite1",     // X11 Composite extension
	"libxdamage1",        // X11 Damage extension
	"libxfixes3",         // X11 Fixes extension
	"libxrandr2",         // X11 RandR extension
	"libgbm1",            // Mesa GBM (buffer management)
	"libxkbcommon0",      // XKB keyboard handling
	"libpango-1.0-0",     // Text rendering
	"libcairo2",          // 2D graphics
	"libasound2",         // ALSA audio
}

// dnfBuildPackages are the RHEL/Fedora/Amazon Linux packages required to compile UE5.
var dnfBuildPackages = []string{
	"gcc",
	"gcc-c++",
	"make",
	"git",
	"cmake",
	"python3",
	"curl",
	"xdg-user-dirs",
	"shared-mime-info",
	"fontconfig-devel",
	"freetype-devel",
	"glibc-devel",
}

// dnfRuntimePackages are the RHEL/Fedora equivalents of AptRuntimePackages.
var dnfRuntimePackages = []string{
	"nss",
	"nspr",
	"dbus-libs",
	"atk",
	"at-spi2-atk",
	"libdrm",
	"libXcomposite",
	"libXdamage",
	"libXfixes",
	"libXrandr",
	"mesa-libgbm",
	"libxkbcommon",
	"pango",
	"cairo",
	"alsa-lib",
}

// installDepsSnippet returns the RUN block that installs UE5 build and runtime prerequisites.
// Used in both the engine Dockerfile (deps stage + runtime stage) and shared between
// the full 5-stage and prebuilt 2-stage Dockerfiles.
//
// Both build and runtime packages are installed because BuildCookRun invokes
// compilers/linkers during game builds AND needs runtime libs for the cook step.
func installDepsSnippet() string {
	aptAll := make([]string, 0, len(aptBuildPackages)+len(AptRuntimePackages))
	aptAll = append(aptAll, aptBuildPackages...)
	aptAll = append(aptAll, AptRuntimePackages...)

	dnfAll := make([]string, 0, len(dnfBuildPackages)+len(dnfRuntimePackages))
	dnfAll = append(dnfAll, dnfBuildPackages...)
	dnfAll = append(dnfAll, dnfRuntimePackages...)

	aptLines := formatPackageList(aptAll, 12)
	dnfLines := formatPackageList(dnfAll, 12)

	return fmt.Sprintf(`RUN set -e; \
    if command -v apt-get >/dev/null 2>&1; then \
        export DEBIAN_FRONTEND=noninteractive; \
        apt-get update && apt-get install -y \
%s \
        && rm -rf /var/lib/apt/lists/*; \
    elif command -v dnf >/dev/null 2>&1; then \
        dnf install -y \
%s \
        && dnf clean all; \
    else \
        echo "ERROR: No supported package manager found (need apt-get or dnf)" >&2; \
        exit 1; \
    fi`, aptLines, dnfLines)
}

// RuntimeDepsInstallScript returns a shell snippet that installs the runtime
// libraries if they are missing. Used in game build preambles to patch older
// engine images that were built before these dependencies were added.
// NOTE: Only handles apt-get (Debian/Ubuntu) images. DNF-based images should
// be rebuilt with the updated engine Dockerfile instead.
func RuntimeDepsInstallScript() string {
	pkgs := strings.Join(AptRuntimePackages, " ")
	return fmt.Sprintf(`if ! ldconfig -p 2>/dev/null | grep -q libnss3; then
    echo "Installing missing runtime dependencies for UnrealEditor-Cmd"
    apt-get update -qq || { echo "ERROR: Failed to update package lists" >&2; exit 1; }
    apt-get install -y -qq \
        %s \
        || { echo "ERROR: Failed to install runtime dependencies" >&2; exit 1; }
    rm -rf /var/lib/apt/lists/*
fi
`, pkgs)
}

// formatPackageList formats a slice of package names as indented continuation lines
// for a shell command (backslash-continuation style).
func formatPackageList(pkgs []string, indent int) string {
	prefix := strings.Repeat(" ", indent)
	var lines []string
	for _, pkg := range pkgs {
		lines = append(lines, prefix+pkg+" \\")
	}
	// Remove trailing backslash from last line.
	if len(lines) > 0 {
		last := lines[len(lines)-1]
		lines[len(lines)-1] = strings.TrimSuffix(last, " \\")
	}
	return strings.Join(lines, "\n")
}
