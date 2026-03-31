//go:build windows

package prereq

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/devrecon/ludus/internal/toolchain"
)

// vswhereResult holds the fields we care about from vswhere JSON output.
type vswhereResult struct {
	DisplayName      string `json:"displayName"`
	InstallationPath string `json:"installationPath"`
}

func (c *Checker) checkVisualStudio() CheckResult {
	vswherePath := filepath.Join(
		os.Getenv("ProgramFiles(x86)"),
		"Microsoft Visual Studio", "Installer", "vswhere.exe",
	)

	if _, err := os.Stat(vswherePath); os.IsNotExist(err) {
		return CheckResult{
			Name:    "Visual Studio",
			Passed:  false,
			Message: "vswhere.exe not found; install Visual Studio with C++ workloads",
		}
	}

	// Check for any VS installation
	out, err := exec.Command(vswherePath, "-format", "json", "-utf8").Output()
	if err != nil {
		return CheckResult{
			Name:    "Visual Studio",
			Passed:  false,
			Message: fmt.Sprintf("vswhere failed: %v", err),
		}
	}

	var installs []vswhereResult
	if err := json.Unmarshal(out, &installs); err != nil || len(installs) == 0 {
		return CheckResult{
			Name:    "Visual Studio",
			Passed:  false,
			Message: "no Visual Studio installation detected",
		}
	}

	edition := installs[0].DisplayName

	// Determine which MSVC toolchain component is required for this engine version.
	// UE 5.4–5.6 need MSVC 14.38; UE 5.7+ need MSVC 14.44.
	msvcCompID := "Microsoft.VisualStudio.Component.VC.14.38.17.8.x86.x64"
	msvcCompName := "MSVC v14.38"
	if needsNewerMSVC(c.EngineSourcePath, c.EngineVersion) {
		msvcCompID = "Microsoft.VisualStudio.Component.VC.14.44.17.14.x86.x64"
		msvcCompName = "MSVC v14.44"
	}

	// Check required components. We use individual component IDs rather than
	// workload IDs (NativeDesktop, NativeGame) because workload IDs are not
	// reliably detected across all VS versions (e.g., VS 2026 doesn't report
	// them via vswhere).
	requiredComponents := []struct {
		id   string
		name string
	}{
		{"Microsoft.VisualStudio.Component.VC.Tools.x86.x64", "C++ build tools"},
		{msvcCompID, msvcCompName},
	}

	var missing []string
	for _, comp := range requiredComponents {
		compOut, compErr := exec.Command(vswherePath,
			"-requires", comp.id,
			"-format", "json", "-utf8",
		).Output()
		if compErr != nil {
			missing = append(missing, comp.name)
			continue
		}
		var compInstalls []vswhereResult
		if json.Unmarshal(compOut, &compInstalls) != nil || len(compInstalls) == 0 {
			missing = append(missing, comp.name)
		}
	}

	if len(missing) > 0 {
		if !c.Fix {
			return CheckResult{
				Name:   "Visual Studio",
				Passed: false,
				Message: fmt.Sprintf("%s found but missing components: %s; "+
					"run with --fix to install them",
					edition, strings.Join(missing, ", ")),
			}
		}

		// Auto-fix: launch VS Installer to add missing components
		setupPath := filepath.Join(
			os.Getenv("ProgramFiles(x86)"),
			"Microsoft Visual Studio", "Installer", "setup.exe",
		)
		if _, err := os.Stat(setupPath); os.IsNotExist(err) {
			return CheckResult{
				Name:   "Visual Studio",
				Passed: false,
				Message: fmt.Sprintf("VS Installer setup.exe not found at %s; install components manually via VS Installer > Modify",
					setupPath),
			}
		}

		// Build the list of missing component IDs
		missingIDs := make(map[string]string)
		for _, comp := range requiredComponents {
			for _, m := range missing {
				if comp.name == m {
					missingIDs[comp.id] = comp.name
				}
			}
		}

		// Build the setup.exe argument string. The --passive flag requires
		// the installer to run elevated, so we use PowerShell Start-Process
		// -Verb RunAs to trigger UAC. Users will see a UAC prompt.
		var setupArgs []string
		setupArgs = append(setupArgs, "modify", "--installPath", installs[0].InstallationPath)
		for id := range missingIDs {
			setupArgs = append(setupArgs, "--add", id)
		}
		setupArgs = append(setupArgs, "--passive", "--norestart")

		// Escape the argument list for PowerShell.
		psArgs := fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '%s' -Verb RunAs -Wait",
			setupPath, strings.Join(setupArgs, " "))

		cmd := exec.Command("powershell", "-NoProfile", "-Command", psArgs)
		if err := cmd.Start(); err != nil {
			return CheckResult{
				Name:    "Visual Studio",
				Passed:  false,
				Message: fmt.Sprintf("failed to launch VS Installer: %v", err),
			}
		}

		return CheckResult{
			Name:    "Visual Studio",
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("launched VS Installer (UAC prompt required) to add: %s; re-run ludus init after installation completes",
				strings.Join(missing, ", ")),
		}
	}

	return CheckResult{
		Name:    "Visual Studio",
		Passed:  true,
		Message: fmt.Sprintf("%s with C++ tools and %s", edition, msvcCompName),
	}
}

// needsNewerMSVC returns true when the engine version is 5.7 or later,
// which requires MSVC 14.44+ and would reject the 14.38 pin.
func needsNewerMSVC(sourcePath, configVersion string) bool {
	ver, _ := toolchain.DetectEngineVersion(sourcePath, configVersion)
	if ver == "" {
		return false // unknown version — 14.38 pin is the safe default
	}
	parts := strings.SplitN(ver, ".", 2)
	if len(parts) < 2 {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return minor >= 7
}

// msvcVersionForEngine returns the MSVC version string to pin in
// BuildConfiguration.xml based on the engine version. UE 5.4–5.6 use
// 14.38.33130; UE 5.7+ use 14.44.35207.
func msvcVersionForEngine(sourcePath, configVersion string) string {
	if needsNewerMSVC(sourcePath, configVersion) {
		return "14.44.35207"
	}
	return "14.38.33130"
}

// buildConfigXMLFor generates a BuildConfiguration.xml body. When compiler
// is non-empty (e.g. "VisualStudio2026"), a <Compiler> tag is included so
// UBT picks the correct installation instead of defaulting to VS 2022.
func buildConfigXMLFor(msvcVersion, compiler string) string {
	compilerLine := ""
	if compiler != "" {
		compilerLine = fmt.Sprintf("\n    <Compiler>%s</Compiler>", compiler)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<Configuration xmlns="https://www.unrealengine.com/BuildConfiguration">
  <WindowsPlatform>%s
    <CompilerVersion>%s</CompilerVersion>
  </WindowsPlatform>
</Configuration>
`, compilerLine, msvcVersion)
}

func (c *Checker) checkMSVCToolchainConfig() CheckResult {
	wantVersion := msvcVersionForEngine(c.EngineSourcePath, c.EngineVersion)
	versionTag := "<CompilerVersion>" + wantVersion + "</CompilerVersion>"

	// UE 5.7+ supports VS 2026 natively — set the Compiler tag so UBT
	// picks VS 2026 instead of defaulting to VS 2022 (which may not exist).
	wantCompiler := ""
	if needsNewerMSVC(c.EngineSourcePath, c.EngineVersion) {
		wantCompiler = "VisualStudio2026"
	}

	configDir := filepath.Join(os.Getenv("APPDATA"), "Unreal Engine", "UnrealBuildTool")
	configPath := filepath.Join(configDir, "BuildConfiguration.xml")

	data, err := os.ReadFile(configPath)
	if err == nil {
		content := string(data)
		hasVersion := strings.Contains(content, versionTag)
		hasCompiler := wantCompiler == "" || strings.Contains(content, "<Compiler>"+wantCompiler+"</Compiler>")
		if hasVersion && hasCompiler {
			msg := fmt.Sprintf("BuildConfiguration.xml pins MSVC %s", wantVersion)
			if wantCompiler != "" {
				msg += fmt.Sprintf(" with Compiler=%s", wantCompiler)
			}
			return CheckResult{
				Name:    "MSVC Toolchain Config",
				Passed:  true,
				Message: msg + " (" + configPath + ")",
			}
		}
	}

	if !c.Fix {
		hint := "file missing"
		if err == nil {
			hint = fmt.Sprintf("CompilerVersion not set to %s", wantVersion)
			if wantCompiler != "" && !strings.Contains(string(data), "<Compiler>"+wantCompiler+"</Compiler>") {
				hint = fmt.Sprintf("Compiler/CompilerVersion not set for %s", wantVersion)
			}
		}
		return CheckResult{
			Name:   "MSVC Toolchain Config",
			Passed: false,
			Message: fmt.Sprintf("%s at %s; run with --fix to create/update it",
				hint, configPath),
		}
	}

	// Auto-fix: write the config file with the correct MSVC version.
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return CheckResult{
			Name:    "MSVC Toolchain Config",
			Passed:  false,
			Message: fmt.Sprintf("failed to create directory %s: %v", configDir, err),
		}
	}

	if err := os.WriteFile(configPath, []byte(buildConfigXMLFor(wantVersion, wantCompiler)), 0o644); err != nil {
		return CheckResult{
			Name:    "MSVC Toolchain Config",
			Passed:  false,
			Message: fmt.Sprintf("failed to write %s: %v", configPath, err),
		}
	}

	msg := fmt.Sprintf("created %s (pins MSVC %s", configPath, wantVersion)
	if wantCompiler != "" {
		msg += ", Compiler=" + wantCompiler
	}
	msg += ")"
	return CheckResult{
		Name:    "MSVC Toolchain Config",
		Passed:  true,
		Message: msg,
	}
}
