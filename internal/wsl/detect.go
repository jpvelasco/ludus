package wsl

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/runner"
)

// Info holds the result of WSL2 detection.
type Info struct {
	Available  bool
	Distros    []DistroInfo
	DiskFreeGB float64
	HasRsync   bool
}

// DistroInfo describes a single WSL distro.
type DistroInfo struct {
	Name    string
	Version int
	Running bool
	Default bool
}

// Detect checks whether WSL2 is available and enumerates installed distros.
func Detect() (*Info, error) {
	if _, err := exec.LookPath(wslExe); err != nil {
		return &Info{Available: false}, nil
	}

	out, err := exec.Command(wslExe, "--list", "--verbose").CombinedOutput()
	if err != nil {
		return &Info{Available: false}, nil
	}

	distros := parseDistroList(string(out))
	info := &Info{
		Available: len(distros) > 0,
		Distros:   distros,
	}
	return info, nil
}

// PickDistro selects the WSL2 distro to use. If override is non-empty, it must
// match an installed WSL2 distro. Otherwise the first running WSL2 distro is picked.
func PickDistro(info *Info, override string) (string, error) {
	if override != "" {
		return findOverrideDistro(info, override)
	}
	return findBestDistro(info)
}

func findOverrideDistro(info *Info, override string) (string, error) {
	for _, d := range info.Distros {
		if strings.EqualFold(d.Name, override) {
			if d.Version != 2 {
				return "", fmt.Errorf("distro %q is WSL%d, not WSL2", d.Name, d.Version)
			}
			return d.Name, nil
		}
	}
	return "", fmt.Errorf("WSL2 distro %q not found; installed: %s", override, distroNames(info.Distros))
}

// findBestDistro prefers running WSL2 distros, then falls back to any WSL2 distro.
func findBestDistro(info *Info) (string, error) {
	for _, d := range info.Distros {
		if d.Version == 2 && d.Running {
			return d.Name, nil
		}
	}
	for _, d := range info.Distros {
		if d.Version == 2 {
			return d.Name, nil
		}
	}
	return "", fmt.Errorf("no WSL2 distro found; install one with: wsl --install")
}

// CheckDeps verifies that essential build dependencies are available in the distro.
func CheckDeps(ctx context.Context, r *runner.Runner, distro string) error {
	required := []string{"gcc", "make", "python3", "cmake"}
	var missing []string
	for _, cmd := range required {
		if err := CheckCommand(ctx, r, distro, cmd); err != nil {
			missing = append(missing, cmd)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing build dependencies in WSL2 distro %q: %s\n"+
			"Install with: wsl -d %s sudo apt-get install -y build-essential cmake python3",
			distro, strings.Join(missing, ", "), distro)
	}
	return nil
}

// InstallDeps installs build AND runtime dependencies in the WSL2 distro.
// Runtime packages (libnss3, libdbus, etc.) are needed by UnrealEditor-Cmd
// during the cook step, even in headless/server mode.
func InstallDeps(ctx context.Context, r *runner.Runner, distro string) error {
	return RunBash(ctx, r, distro, installDepsScript())
}

// installDepsScript returns the shell script that installs all UE5 dependencies.
// Separated for testability.
func installDepsScript() string {
	runtimePkgs := strings.Join(dockerbuild.AptRuntimePackages, " ")
	return "export DEBIAN_FRONTEND=noninteractive && " +
		"sudo apt-get update && " +
		"sudo apt-get install -y " +
		"build-essential git cmake python3 curl rsync " +
		"xdg-user-dirs shared-mime-info " +
		"libfontconfig1 libfreetype6 libc6-dev " +
		runtimePkgs
}

// CheckRuntimeDeps verifies that runtime libraries needed by UnrealEditor-Cmd
// are present in the distro. Uses libnss3 as a sentinel (same check as the
// container build preamble in dockerbuild.RuntimeDepsInstallScript).
func CheckRuntimeDeps(ctx context.Context, r *runner.Runner, distro string) error {
	_, err := RunOutput(ctx, r, distro, "bash", "-c", "ldconfig -p 2>/dev/null | grep -q libnss3")
	if err != nil {
		return fmt.Errorf("runtime libraries missing in WSL2 distro %q; "+
			"needed by UnrealEditor-Cmd for cooking", distro)
	}
	return nil
}

// InstallRuntimeDeps installs only the runtime dependencies needed by
// UnrealEditor-Cmd. Used as a safety net in game builds when the engine
// was built before runtime deps were added to InstallDeps.
func InstallRuntimeDeps(ctx context.Context, r *runner.Runner, distro string) error {
	return RunBash(ctx, r, distro, installRuntimeDepsScript())
}

// installRuntimeDepsScript returns the shell script for runtime-only deps.
func installRuntimeDepsScript() string {
	pkgs := strings.Join(dockerbuild.AptRuntimePackages, " ")
	return "export DEBIAN_FRONTEND=noninteractive && " +
		"sudo apt-get update -qq && " +
		"sudo apt-get install -y -qq " + pkgs
}

// CheckDiskSpace returns the free disk space in GB on the distro's root filesystem.
func CheckDiskSpace(ctx context.Context, r *runner.Runner, distro string) (float64, error) {
	out, err := RunOutput(ctx, r, distro, "df", "-BG", "--output=avail", "/")
	if err != nil {
		return 0, fmt.Errorf("checking disk space: %w", err)
	}
	return parseDiskFreeGB(string(out))
}

// CheckRsync checks if rsync is available in the distro.
func CheckRsync(ctx context.Context, r *runner.Runner, distro string) bool {
	return CheckCommand(ctx, r, distro, "rsync") == nil
}

// parseDistroList parses the output of `wsl --list --verbose`.
// The output format is:
//
//	  NAME            STATE           VERSION
//	* Ubuntu          Running         2
//	  Debian          Stopped         2
func parseDistroList(output string) []DistroInfo {
	lines := strings.Split(output, "\n")
	var distros []DistroInfo

	for _, line := range lines {
		// Strip BOM and NUL bytes (wsl.exe outputs UTF-16LE).
		line = cleanWSLOutput(line)
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(strings.ToUpper(line), "NAME") {
			continue
		}

		d, ok := parseDistroLine(line)
		if ok {
			distros = append(distros, d)
		}
	}
	return distros
}

// parseDistroLine parses a single line from `wsl --list --verbose` output.
func parseDistroLine(line string) (DistroInfo, bool) {
	isDefault := strings.HasPrefix(line, "*")
	line = strings.TrimPrefix(line, "*")
	line = strings.TrimSpace(line)

	fields := strings.Fields(line)
	if len(fields) < 3 {
		return DistroInfo{}, false
	}

	version, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil {
		return DistroInfo{}, false
	}

	state := fields[len(fields)-2]
	// Name can contain spaces — it's everything before state and version.
	name := strings.Join(fields[:len(fields)-2], " ")

	return DistroInfo{
		Name:    name,
		Version: version,
		Running: strings.EqualFold(state, "Running"),
		Default: isDefault,
	}, true
}

// cleanWSLOutput strips BOM, NUL bytes, and carriage returns from wsl.exe output.
// wsl.exe on Windows outputs UTF-16LE which Go reads as bytes with embedded NULs.
func cleanWSLOutput(s string) string {
	// Remove BOM (UTF-8: EF BB BF, or UTF-16LE: FF FE).
	s = strings.TrimPrefix(s, "\xef\xbb\xbf")
	s = strings.TrimPrefix(s, "\xff\xfe")
	// Strip NUL bytes (from UTF-16LE read as bytes).
	s = strings.ReplaceAll(s, "\x00", "")
	// Normalize line endings.
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// parseDiskFreeGB parses df -BG output to extract the available GB value.
func parseDiskFreeGB(output string) (float64, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "Avail") {
			continue
		}
		// Value looks like "123G" or "123".
		line = strings.TrimSuffix(line, "G")
		val, err := strconv.ParseFloat(line, 64)
		if err != nil {
			continue
		}
		return val, nil
	}
	return 0, fmt.Errorf("could not parse disk space from: %s", output)
}

// distroNames returns a comma-separated list of distro names.
func distroNames(distros []DistroInfo) string {
	names := make([]string, len(distros))
	for i, d := range distros {
		names[i] = d.Name
	}
	return strings.Join(names, ", ")
}
