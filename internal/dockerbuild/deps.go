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
	"libicu-dev",     // required by dotnet (UBT/UHT) on ARM64 Ubuntu; x86_64 gets it transitively
	"dotnet-sdk-8.0", // required for UBT (UnrealBuildTool) on Ubuntu 22.04; fixes 'System.Runtime.Numerics not found'
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

// AptRuntimeT64Packages maps Ubuntu 22.04 package names to their Ubuntu 24.04+
// equivalents where the 64-bit time_t transition renamed the package.
// Used by the WSL2 backend which may run any Ubuntu version.
var AptRuntimeT64Packages = map[string]string{
	"libatk1.0-0":        "libatk1.0-0t64",
	"libatk-bridge2.0-0": "libatk-bridge2.0-0t64",
	"libasound2":         "libasound2t64",
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
	"dotnet-sdk-8.0", // required for UBT on dnf-based images (Amazon Linux etc.)
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

	// UE5 ships a bundled .NET SDK under Engine/Binaries/ThirdParty/DotNet/ for all
	// platforms including linux-arm64. The system dotnet-sdk-8.0 was only added to fix
	// a missing System.Runtime.Numerics on amd64 Ubuntu 22.04 where the MS apt repo is
	// available. On arm64 the MS repo does not carry dotnet-sdk-8.0 for jammy, and the
	// bundled UE .NET handles UBT/UHT correctly without it.
	aptAllArm64 := make([]string, 0, len(aptAll))
	for _, pkg := range aptAll {
		if pkg != "dotnet-sdk-8.0" {
			aptAllArm64 = append(aptAllArm64, pkg)
		}
	}

	dnfAll := make([]string, 0, len(dnfBuildPackages)+len(dnfRuntimePackages))
	dnfAll = append(dnfAll, dnfBuildPackages...)
	dnfAll = append(dnfAll, dnfRuntimePackages...)

	aptLinesAmd64 := formatPackageList(aptAll, 16)
	aptLinesArm64 := formatPackageList(aptAllArm64, 16)
	dnfLines := formatPackageList(dnfAll, 12)

	return fmt.Sprintf(`RUN set -e; \
    if command -v apt-get >/dev/null 2>&1; then \
        export DEBIAN_FRONTEND=noninteractive; \
        apt-get update && apt-get install -y wget apt-transport-https ca-certificates; \
        ARCH=$(dpkg --print-architecture); \
        if [ "$ARCH" = "amd64" ]; then \
            wget -q https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb \
                -O /tmp/packages-microsoft-prod.deb; \
            dpkg -i /tmp/packages-microsoft-prod.deb; \
            rm /tmp/packages-microsoft-prod.deb; \
            apt-get update && apt-get install -y \
%s \
            && rm -rf /var/lib/apt/lists/*; \
        else \
            apt-get install -y \
%s \
            && rm -rf /var/lib/apt/lists/*; \
        fi; \
    elif command -v dnf >/dev/null 2>&1; then \
        dnf install -y \
%s \
        && dnf clean all; \
    else \
        echo "ERROR: No supported package manager found (need apt-get or dnf)" >&2; \
        exit 1; \
    fi`, aptLinesAmd64, aptLinesArm64, dnfLines)
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

// PreflightDepsInstallScript returns a shell one-liner that installs UE5 build
// prerequisites using the image's package manager (apt-get or dnf). Used in
// macOS pre-flight containers that run against a bare base image before the
// main engine Dockerfile build starts.
func PreflightDepsInstallScript() string {
	aptPkgs := strings.Join(aptBuildPackages, " ")
	dnfPkgs := strings.Join(dnfBuildPackages, " ")
	return fmt.Sprintf(
		`if command -v apt-get >/dev/null 2>&1; then `+
			`DEBIAN_FRONTEND=noninteractive apt-get update -qq && apt-get install -y --no-install-recommends ca-certificates wget apt-transport-https && `+
			`wget -q https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb -O /tmp/packages-microsoft-prod.deb && `+
			`dpkg -i /tmp/packages-microsoft-prod.deb && rm /tmp/packages-microsoft-prod.deb && `+
			`apt-get update -qq && apt-get install -y --no-install-recommends %s && rm -rf /var/lib/apt/lists/*; `+
			`elif command -v dnf >/dev/null 2>&1; then `+
			`dnf install -y %s && dnf clean all; `+
			`else echo "ERROR: no supported package manager (need apt-get or dnf)" >&2; exit 1; fi`,
		aptPkgs, dnfPkgs,
	)
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
