package game

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// resolveMaxJobs returns the effective parallel compile job limit. If MaxJobs
// is explicitly set (> 0), it is returned as-is. Otherwise, auto-detects from
// available RAM: 8 GB per job for native builds, 16 GB per job for cross-compile
// (both Win64 and Linux toolchains loaded simultaneously). Returns 0 if RAM
// cannot be detected (lets UBT decide internally).
func (b *Builder) resolveMaxJobs(crossCompile bool) int {
	if b.opts.MaxJobs > 0 {
		return b.opts.MaxJobs
	}

	ramGB := totalRAMGB()
	if ramGB == 0 {
		return 0
	}

	gbPerJob := 8
	if crossCompile {
		gbPerJob = 16
	}

	return max(ramGB/gbPerJob, 1)
}

// applyNuGetAuditWorkaround sets NuGetAuditLevel=critical as an environment
// variable on the runner's child processes. UE 5.6's Gauntlet test framework
// directly depends on Magick.NET 14.7.0 which has known low/moderate/high
// CVEs. Combined with Epic's TreatWarningsAsErrors, this causes AutomationTool
// script modules to fail to compile. MSBuild reads NuGetAuditLevel from the
// environment, so this avoids writing a Directory.Build.props into the engine
// source tree. Only applied for engine 5.6 or when the version is unknown
// (safe default — the env var is harmless on other versions).
func (b *Builder) applyNuGetAuditWorkaround() {
	v := b.opts.EngineVersion
	if v != "" && v != "5.6" {
		return
	}
	b.Runner.Env = append(b.Runner.Env, "NuGetAuditLevel=critical")
}

// ensureLinuxMultiarchRoot explicitly passes LINUX_MULTIARCH_ROOT through the
// runner's Env so it survives RunUAT's AutoSDK environment manipulation.
// RunUAT's Turnkey system can strip or reset this variable when switching SDK
// modes, which prevents UnrealEditor-Cmd.exe from detecting the Linux platform
// during content cooking. By setting it on the runner, it's merged into every
// child process environment regardless of what RunUAT does internally.
//
// On Windows, if the env var is not set in the current process (common after
// toolchain install without restarting the terminal), falls back to reading
// the system environment from the Windows registry.
func (b *Builder) ensureLinuxMultiarchRoot() {
	v := os.Getenv("LINUX_MULTIARCH_ROOT")
	if v == "" && runtime.GOOS == "windows" {
		v = readSystemEnvVar("LINUX_MULTIARCH_ROOT")
	}
	if v != "" {
		b.Runner.Env = append(b.Runner.Env, "LINUX_MULTIARCH_ROOT="+v)
	}
}

// ensureDefaultServerTarget adds DefaultServerTarget to the project's
// DefaultEngine.ini if not already set. UE 5.6 Lyra ships with multiple
// server targets and RunUAT refuses to build without this setting, even
// when -servertargetname is passed on the command line.
func (b *Builder) ensureDefaultServerTarget(projectPath string) error {
	iniPath := filepath.Join(filepath.Dir(projectPath), "Config", "DefaultEngine.ini")

	data, err := os.ReadFile(iniPath)
	if err != nil {
		// If the INI doesn't exist, skip gracefully — non-Lyra projects may not need this
		if os.IsNotExist(err) {
			fmt.Printf("  %s not found, skipping DefaultServerTarget configuration\n", iniPath)
			return nil
		}
		return fmt.Errorf("reading %s: %w", iniPath, err)
	}

	content := string(data)
	if strings.Contains(content, "DefaultServerTarget") {
		return nil
	}

	old := "DefaultGameTarget=" + b.gameTargetName()
	replacement := old + "\nDefaultServerTarget=" + b.serverTargetName()
	if !strings.Contains(content, old) {
		fmt.Printf("  %s does not contain DefaultGameTarget=%s, skipping DefaultServerTarget configuration\n", iniPath, b.gameTargetName())
		return nil
	}

	content = strings.Replace(content, old, replacement, 1)
	fmt.Printf("  Setting DefaultServerTarget=%s in %s\n", b.serverTargetName(), iniPath)
	return os.WriteFile(iniPath, []byte(content), 0644)
}

func (b *Builder) gameTargetName() string {
	if b.opts.GameTarget != "" {
		return b.opts.GameTarget
	}
	return b.opts.ProjectName + "Game"
}

// ensureTargetArchitecture sets TargetArchitecture=AArch64 in the project's
// DefaultEngine.ini for ARM64 builds. UE5 requires this setting under
// [/Script/LinuxTargetPlatform.LinuxTargetSettings] to target ARM64.
func (b *Builder) ensureTargetArchitecture(projectPath string) error {
	iniPath := filepath.Join(filepath.Dir(projectPath), "Config", "DefaultEngine.ini")

	data, err := os.ReadFile(iniPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("  %s not found, skipping TargetArchitecture configuration\n", iniPath)
			return nil
		}
		return fmt.Errorf("reading %s: %w", iniPath, err)
	}

	return b.writeTargetArchitecture(iniPath, string(data))
}

func (b *Builder) writeTargetArchitecture(iniPath, content string) error {
	entry := "TargetArchitecture=AArch64"
	if strings.Contains(content, entry) {
		return nil
	}

	content = addTargetArchitecture(content, entry)
	fmt.Printf("  Setting %s in %s\n", entry, iniPath)
	return os.WriteFile(iniPath, []byte(content), 0644)
}

func addTargetArchitecture(content, entry string) string {
	section := "[/Script/LinuxTargetPlatform.LinuxTargetSettings]"
	if strings.Contains(content, section) {
		return strings.Replace(content, section, section+"\n"+entry, 1)
	}
	return content + "\n" + section + "\n" + entry + "\n"
}

// disableDumpSyms adds bDisableDumpSyms=true to BuildConfiguration.xml for ARM64
// cross-compiled builds on Windows. dump_syms.exe crashes with out-of-memory errors
// when processing large ARM64 debug files (1+ GB), causing the build to fail with
// "AddBuildProductsFromManifest...sym...could not be found". The .sym (Breakpad)
// file is not needed for dedicated server operation — the .debug (DWARF) file is
// still generated for post-mortem analysis.
//
// Returns a restore function that should be deferred to restore the original file.
func (b *Builder) disableDumpSyms() func() {
	if runtime.GOOS != "windows" {
		return func() {}
	}

	configPath := filepath.Join(os.Getenv("APPDATA"), "Unreal Engine", "UnrealBuildTool", "BuildConfiguration.xml")
	original, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("  Warning: could not read %s to disable dump_syms: %v\n", configPath, err)
		return func() {}
	}

	return b.disableDumpSymsInConfig(configPath, original)
}

func (b *Builder) disableDumpSymsInConfig(configPath string, original []byte) func() {
	content, ok := dumpSymsDisabledContent(string(original))
	if !ok {
		fmt.Printf("  Warning: unrecognized BuildConfiguration.xml format, cannot disable dump_syms\n")
		return func() {}
	}
	if content == string(original) {
		return func() {}
	}

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		fmt.Printf("  Warning: could not update %s: %v\n", configPath, err)
		return func() {}
	}

	fmt.Println("  Disabled dump_syms in BuildConfiguration.xml (ARM64 cross-compile workaround)")
	return restoreFile(configPath, original)
}

func dumpSymsDisabledContent(content string) (string, bool) {
	tag := "<bDisableDumpSyms>true</bDisableDumpSyms>"
	switch {
	case strings.Contains(content, tag):
		return content, true
	case strings.Contains(content, "<BuildConfiguration>"):
		return strings.Replace(content, "<BuildConfiguration>",
			"<BuildConfiguration>\n    "+tag, 1), true
	case strings.Contains(content, "</Configuration>"):
		return strings.Replace(content, "</Configuration>",
			"  <BuildConfiguration>\n    "+tag+"\n  </BuildConfiguration>\n</Configuration>", 1), true
	default:
		return "", false
	}
}

func restoreFile(path string, original []byte) func() {
	return func() {
		if err := os.WriteFile(path, original, 0644); err != nil {
			fmt.Printf("  Warning: could not restore %s: %v\n", path, err)
		}
	}
}
