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

// vsComponent describes a required Visual Studio component.
type vsComponent struct {
	id   string
	name string
}

func (c *Checker) checkVisualStudio() CheckResult {
	vswherePath := filepath.Join(
		os.Getenv("ProgramFiles(x86)"),
		"Microsoft Visual Studio", "Installer", "vswhere.exe",
	)

	installs, err := findVSInstalls(vswherePath)
	if err != nil {
		return CheckResult{Name: "Visual Studio", Passed: false, Message: err.Error()}
	}

	msvcCompID := "Microsoft.VisualStudio.Component.VC.14.38.17.8.x86.x64"
	msvcCompName := "MSVC v14.38"
	if needsNewerMSVC(c.EngineSourcePath, c.EngineVersion) {
		msvcCompID = "Microsoft.VisualStudio.Component.VC.14.44.17.14.x86.x64"
		msvcCompName = "MSVC v14.44"
	}

	required := []vsComponent{
		{"Microsoft.VisualStudio.Component.VC.Tools.x86.x64", "C++ build tools"},
		{msvcCompID, msvcCompName},
	}

	missing := findMissingComponents(vswherePath, required)
	edition := installs[0].DisplayName

	if len(missing) == 0 {
		return CheckResult{
			Name:    "Visual Studio",
			Passed:  true,
			Message: fmt.Sprintf("%s with C++ tools and %s", edition, msvcCompName),
		}
	}

	if !c.Fix {
		return CheckResult{
			Name:   "Visual Studio",
			Passed: false,
			Message: fmt.Sprintf("%s found but missing components: %s; "+
				"run with --fix to install them",
				edition, strings.Join(missing, ", ")),
		}
	}

	return c.fixMissingVSComponents(installs[0], missing, required)
}

// findVSInstalls returns installed Visual Studio instances, or an error.
func findVSInstalls(vswherePath string) ([]vswhereResult, error) {
	if _, err := os.Stat(vswherePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("vswhere.exe not found; install Visual Studio with C++ workloads")
	}

	out, err := exec.Command(vswherePath, "-format", "json", "-utf8").Output()
	if err != nil {
		return nil, fmt.Errorf("vswhere failed: %v", err)
	}

	var installs []vswhereResult
	if err := json.Unmarshal(out, &installs); err != nil || len(installs) == 0 {
		return nil, fmt.Errorf("no Visual Studio installation detected")
	}
	return installs, nil
}

// findMissingComponents checks which required VS components are not installed.
func findMissingComponents(vswherePath string, required []vsComponent) []string {
	var missing []string
	for _, comp := range required {
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
	return missing
}

// fixMissingVSComponents launches the VS Installer to add missing components.
func (c *Checker) fixMissingVSComponents(install vswhereResult, missing []string, required []vsComponent) CheckResult {
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

	// Collect missing component IDs
	missingSet := make(map[string]bool, len(missing))
	for _, m := range missing {
		missingSet[m] = true
	}

	var setupArgs []string
	setupArgs = append(setupArgs, "modify", "--installPath", install.InstallationPath)
	for _, comp := range required {
		if missingSet[comp.name] {
			setupArgs = append(setupArgs, "--add", comp.id)
		}
	}
	setupArgs = append(setupArgs, "--passive", "--norestart")

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

// needsNewerMSVC returns true when the engine version is 5.7 or later,
// which requires MSVC 14.44+ and would reject the 14.38 pin.
func needsNewerMSVC(sourcePath, configVersion string) bool {
	ver, _ := toolchain.DetectEngineVersion(sourcePath, configVersion)
	if ver == "" {
		return false
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
// BuildConfiguration.xml based on the engine version.
func msvcVersionForEngine(sourcePath, configVersion string) string {
	if needsNewerMSVC(sourcePath, configVersion) {
		return "14.44.35207"
	}
	return "14.38.33130"
}

// buildConfigXMLFor generates a BuildConfiguration.xml body.
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

	wantCompiler := ""
	if needsNewerMSVC(c.EngineSourcePath, c.EngineVersion) {
		wantCompiler = "VisualStudio2026"
	}

	configDir := filepath.Join(os.Getenv("APPDATA"), "Unreal Engine", "UnrealBuildTool")
	configPath := filepath.Join(configDir, "BuildConfiguration.xml")

	if ok, msg := checkExistingMSVCConfig(configPath, versionTag, wantVersion, wantCompiler); ok {
		return CheckResult{Name: "MSVC Toolchain Config", Passed: true, Message: msg}
	}

	if !c.Fix {
		return msvcConfigHint(configPath, wantVersion, wantCompiler)
	}

	return fixMSVCConfig(configDir, configPath, wantVersion, wantCompiler)
}

// checkExistingMSVCConfig returns (true, message) if the config is already correct.
func checkExistingMSVCConfig(configPath, versionTag, wantVersion, wantCompiler string) (bool, string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false, ""
	}
	content := string(data)
	if !strings.Contains(content, versionTag) {
		return false, ""
	}
	if wantCompiler != "" && !strings.Contains(content, "<Compiler>"+wantCompiler+"</Compiler>") {
		return false, ""
	}
	msg := fmt.Sprintf("BuildConfiguration.xml pins MSVC %s", wantVersion)
	if wantCompiler != "" {
		msg += fmt.Sprintf(" with Compiler=%s", wantCompiler)
	}
	return true, msg + " (" + configPath + ")"
}

// msvcConfigHint returns a CheckResult explaining what needs fixing.
func msvcConfigHint(configPath, wantVersion, wantCompiler string) CheckResult {
	data, err := os.ReadFile(configPath)
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

// fixMSVCConfig writes the BuildConfiguration.xml with the correct MSVC version.
func fixMSVCConfig(configDir, configPath, wantVersion, wantCompiler string) CheckResult {
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
	return CheckResult{Name: "MSVC Toolchain Config", Passed: true, Message: msg}
}
