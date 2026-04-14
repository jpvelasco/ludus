package game

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/ddc"
	"github.com/devrecon/ludus/internal/progress"
	"github.com/devrecon/ludus/internal/runner"
)

// BuildOptions configures the game server build.
type BuildOptions struct {
	// EnginePath is the path to the built Unreal Engine.
	EnginePath string
	// ProjectPath is the path to the .uproject file.
	ProjectPath string
	// ProjectName is the UE5 project name (e.g. "Lyra", "MyGame").
	ProjectName string
	// ServerTarget is the server build target (e.g. "LyraServer").
	ServerTarget string
	// ClientTarget is the client build target (e.g. "LyraGame").
	ClientTarget string
	// GameTarget is the default game target (e.g. "LyraGame").
	GameTarget string
	// Platform is the target platform (default: "linux").
	Platform string
	// ClientPlatform is the target platform for client builds (default: "Linux").
	// Supported values: "Linux", "Win64".
	ClientPlatform string
	// ServerOnly builds only the server target.
	ServerOnly bool
	// SkipCook skips content cooking.
	SkipCook bool
	// ServerMap is the default map for the dedicated server.
	ServerMap string
	// OutputDir is the archive directory for the packaged build.
	OutputDir string
	// EngineVersion is the detected engine major.minor version (e.g. "5.6").
	// Used to apply version-specific workarounds.
	EngineVersion string
	// Arch is the target CPU architecture: "amd64" (default) or "arm64".
	Arch string
	// ServerConfig is the build configuration for the server (e.g. "Development", "Shipping").
	// Defaults to "Development" if empty.
	ServerConfig string
	// MaxJobs limits parallel compile actions passed to UBT via RunUAT.
	// 0 = auto-detect based on RAM (halved for cross-compile on Windows).
	MaxJobs int
	// DDCMode is the DDC backend mode: "local" or "none".
	DDCMode string
	// DDCPath is the host path for persistent DDC storage.
	DDCPath string
}

// BuildResult holds the outcome of a game server build.
type BuildResult struct {
	// Success indicates whether the build completed.
	Success bool
	// OutputDir is the path to the packaged server build.
	OutputDir string
	// ServerBinary is the path to the server executable.
	ServerBinary string
	// Duration is the build time in seconds.
	Duration float64
	// Error is set if the build failed.
	Error error
}

// Builder handles UE5 dedicated server compilation.
type Builder struct {
	opts   BuildOptions
	Runner *runner.Runner
}

// NewBuilder creates a new game builder.
func NewBuilder(opts BuildOptions, r *runner.Runner) *Builder {
	return &Builder{opts: opts, Runner: r}
}

// resolveRunUAT returns the shell command and RunUAT script path for the current OS.
// On Windows: cmd, RunUAT.bat; on Linux/macOS: bash, RunUAT.sh.
// The returned scriptPath is relative to the engine root (used with RunInDir).
func (b *Builder) resolveRunUAT() (shell, scriptPath string, err error) {
	relPath := filepath.Join("Engine", "Build", "BatchFiles")
	absCheck := filepath.Join(b.opts.EnginePath, relPath)
	if runtime.GOOS == "windows" {
		shell = "cmd"
		scriptPath = filepath.Join(relPath, "RunUAT.bat")
		absCheck = filepath.Join(absCheck, "RunUAT.bat")
	} else {
		shell = "bash"
		scriptPath = filepath.Join(relPath, "RunUAT.sh")
		absCheck = filepath.Join(absCheck, "RunUAT.sh")
	}
	if _, statErr := os.Stat(absCheck); os.IsNotExist(statErr) {
		return "", "", fmt.Errorf("%s not found at %s", filepath.Base(absCheck), absCheck)
	}
	return shell, scriptPath, nil
}

// execRunUAT runs RunUAT with the given arguments using the appropriate shell for the OS.
// scriptPath is relative to the engine root directory (set via RunInDir).
func (b *Builder) execRunUAT(ctx context.Context, shell, scriptPath string, uatArgs []string) error {
	var args []string
	if runtime.GOOS == "windows" {
		args = append([]string{"/c", scriptPath}, uatArgs...)
	} else {
		args = append([]string{scriptPath}, uatArgs...)
	}
	return b.Runner.RunInDir(ctx, b.opts.EnginePath, shell, args...)
}

// LocateProject finds the game project within the engine source tree.
func (b *Builder) LocateProject() (string, error) {
	if b.opts.ProjectPath != "" {
		if _, err := os.Stat(b.opts.ProjectPath); err != nil {
			return "", fmt.Errorf("configured project path not found: %s", b.opts.ProjectPath)
		}
		return b.opts.ProjectPath, nil
	}

	// Auto-detect from engine Samples directory (Lyra-specific default)
	if b.opts.ProjectName == "Lyra" {
		candidate := filepath.Join(b.opts.EnginePath, "Samples", "Games", "Lyra", "Lyra.uproject")
		if _, err := os.Stat(candidate); err != nil {
			return "", fmt.Errorf("Lyra.uproject not found at %s (set game.projectPath in ludus.yaml)", candidate)
		}
		return candidate, nil
	}

	return "", fmt.Errorf("game.projectPath must be set in ludus.yaml for project %q", b.opts.ProjectName)
}

// Build runs the full BuildCookRun pipeline for the game server.
func (b *Builder) Build(ctx context.Context) (*BuildResult, error) {
	start := time.Now()
	result := &BuildResult{}

	projectPath, err := b.LocateProject()
	if err != nil {
		result.Error = err
		return result, err
	}

	shell, runatPath, err := b.resolveRunUAT()
	if err != nil {
		result.Error = err
		return result, err
	}

	if err := b.prepareBuildEnvironment(projectPath); err != nil {
		result.Error = err
		return result, err
	}

	args, outputDir, serverTarget, err := b.resolveServerBuildArgs(projectPath)
	if err != nil {
		result.Error = err
		return result, err
	}
	result.OutputDir = outputDir
	if err := b.setupDDC(); err != nil {
		result.Error = err
		return result, err
	}

	if err := b.runBuildStep(ctx, shell, runatPath, args); err != nil {
		result.Error = err
		return result, err
	}

	arch := config.NormalizeArch(b.opts.Arch)
	result.Success = true
	result.ServerBinary = filepath.Join(outputDir, config.ServerPlatformDir(arch), serverTarget)
	result.Duration = time.Since(start).Seconds()
	return result, nil
}

// prepareBuildEnvironment applies workarounds and ensures ARM64 settings.
func (b *Builder) prepareBuildEnvironment(projectPath string) error {
	b.applyNuGetAuditWorkaround()
	b.ensureLinuxMultiarchRoot()

	if err := b.ensureDefaultServerTarget(projectPath); err != nil {
		return fmt.Errorf("setting default server target: %w", err)
	}

	arch := config.NormalizeArch(b.opts.Arch)
	if arch == "arm64" {
		if err := b.ensureTargetArchitecture(projectPath); err != nil {
			return fmt.Errorf("setting target architecture: %w", err)
		}
		defer b.disableDumpSyms()()
	}
	return nil
}

// setupDDC configures DDC by setting the UE-LocalDataCachePath environment
// variable on the runner. This overrides UE5's default local DDC path without
// modifying any project or engine files. Returns an error if the DDC directory
// cannot be created (permission denied, disk full, etc.).
func (b *Builder) setupDDC() error {
	switch b.opts.DDCMode {
	case ddc.ModeLocal:
		if b.opts.DDCPath == "" {
			return fmt.Errorf("DDC mode is %q but no path configured; set ddc.localPath in ludus.yaml or use --ddc none", ddc.ModeLocal)
		}
		if err := os.MkdirAll(b.opts.DDCPath, 0755); err != nil {
			return fmt.Errorf("creating DDC directory %s: %w", b.opts.DDCPath, err)
		}
		fmt.Printf("  DDC: using persistent cache at %s\n", b.opts.DDCPath)
		b.Runner.Env = append(b.Runner.Env, ddc.EnvOverride(b.opts.DDCPath))
	case ddc.ModeNone, "":
		// No DDC configuration needed.
	default:
		return fmt.Errorf("unsupported DDC mode %q; valid values are %q or %q", b.opts.DDCMode, ddc.ModeLocal, ddc.ModeNone)
	}
	return nil
}

// resolveServerBuildArgs assembles the UAT arguments for BuildCookRun.
// Returns the args slice, resolved output directory, and server target name.
func (b *Builder) resolveServerBuildArgs(projectPath string) ([]string, string, string, error) {
	arch := config.NormalizeArch(b.opts.Arch)
	outputDir := b.opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(filepath.Dir(projectPath), "PackagedServer")
	}

	serverTarget := b.opts.ServerTarget
	if serverTarget == "" {
		serverTarget = b.opts.ProjectName + "Server"
	}

	uePlatform := config.UEPlatformName(arch)
	args := []string{
		"BuildCookRun",
		fmt.Sprintf(`-project="%s"`, projectPath),
		"-platform=" + uePlatform,
		"-server",
		"-noclient",
		fmt.Sprintf("-servertargetname=%s", serverTarget),
		"-build",
		"-stage",
		"-package",
		"-archive",
		fmt.Sprintf(`-archivedirectory="%s"`, outputDir),
	}

	if b.opts.ServerConfig != "" {
		args = append(args, fmt.Sprintf("-serverconfig=%s", b.opts.ServerConfig))
	}

	if !b.opts.SkipCook {
		args = append(args, "-cook")
	} else {
		args = append(args, "-skipcook")
	}

	if b.opts.ServerMap != "" {
		args = append(args, fmt.Sprintf(`-map="%s"`, b.opts.ServerMap))
	}

	// Limit compile parallelism to prevent OOM. During cross-compile (Windows->Linux),
	// both toolchains are loaded simultaneously, roughly doubling memory per job.
	isCrossCompile := runtime.GOOS == "windows" // server is always Linux
	if jobs := b.resolveMaxJobs(isCrossCompile); jobs > 0 {
		args = append(args, fmt.Sprintf("-MaxParallelActions=%d", jobs))
		fmt.Printf("  Limiting parallel compile actions to %d\n", jobs)
	}

	return args, outputDir, serverTarget, nil
}

// runBuildStep executes the UAT build and wraps errors with diagnostics.
func (b *Builder) runBuildStep(ctx context.Context, shell, runatPath string, args []string) error {
	ticker := progress.Start("Server build", 2*time.Minute)
	buildErr := b.execRunUAT(ctx, shell, runatPath, args)
	ticker.Stop()
	if buildErr != nil {
		return diagnoseBuildError(buildErr, "BuildCookRun", b.opts.EnginePath)
	}
	return nil
}

// PartialBuildHint checks for cooked content from a previous server build
// that could be reused with --skip-cook to avoid re-cooking (30-60 min).
// Returns empty string if no partial build is detected or --skip-cook is set.
func (b *Builder) PartialBuildHint() string {
	if b.opts.SkipCook {
		return ""
	}

	projectPath, err := b.LocateProject()
	if err != nil {
		return ""
	}

	projectDir := filepath.Dir(projectPath)
	arch := config.NormalizeArch(b.opts.Arch)
	platformDir := config.ServerPlatformDir(arch)

	cookedDir := filepath.Join(projectDir, "Saved", "Cooked", platformDir)
	if !dirHasContent(cookedDir) {
		return ""
	}

	// Cooked content exists. Check if the final build output is complete.
	outputDir := b.opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(projectDir, "PackagedServer")
	}
	serverTarget := b.opts.ServerTarget
	if serverTarget == "" {
		serverTarget = b.opts.ProjectName + "Server"
	}
	serverBin := filepath.Join(outputDir, platformDir, serverTarget)
	if _, err := os.Stat(serverBin); err == nil {
		return "" // full build already exists
	}

	return fmt.Sprintf("Previous cooked content found at %s\n"+
		"  To skip re-cooking (saves 30-60 min), re-run with: ludus game build --skip-cook", cookedDir)
}

// dirHasContent checks if a directory exists and contains at least one entry.
func dirHasContent(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// diagnoseBuildError inspects a build error for known failure patterns and
// returns an error with actionable guidance. Scans the RunUAT log file for
// common error patterns and appends diagnostics to the error message.
func diagnoseBuildError(err error, action, enginePath string) error {
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()

		// 0xC0E90002 = 3236495362 (uint32→int on 64-bit Go)
		// Windows SmartScreen/UAC blocks freshly-built executables the first
		// time they run. The cook log will be 0 bytes.
		if code == 0xC0E90002 {
			return fmt.Errorf("%s failed (exit code 0xC0E90002): Windows SmartScreen/UAC "+
				"may be blocking a freshly-built executable. Try one of:\n"+
				"  1. Run 'ludus game build' as Administrator (one time only)\n"+
				"  2. Navigate to Engine/Binaries/Win64/ and run UnrealEditor-Cmd.exe manually to approve it\n"+
				"  3. Right-click the blocked .exe -> Properties -> Unblock\n"+
				"Original error: %w", action, err)
		}
	}

	// Scan build logs for additional diagnostics
	var b strings.Builder
	fmt.Fprintf(&b, "%s failed", action)
	if hints := scanBuildLogs(enginePath); len(hints) > 0 {
		b.WriteString("\n\nDiagnostics from build logs:")
		for _, h := range hints {
			b.WriteString("\n  - ")
			b.WriteString(h)
		}
	}

	logDir := filepath.Join(enginePath, "Engine", "Programs", "AutomationTool", "Saved", "Logs")
	fmt.Fprintf(&b, "\n\nFull build log: %s", filepath.Join(logDir, "Log.txt"))

	return fmt.Errorf("%s: %w", b.String(), err)
}

// knownLogPattern maps a string found in build logs to user-facing guidance.
type knownLogPattern struct {
	pattern string
	hint    string
}

// knownLogPatterns is the table of error patterns discovered during E2E testing.
// Each pattern is a substring to search for in the RunUAT log file, paired with
// actionable guidance for the user.
var knownLogPatterns = []knownLogPattern{
	{"GameFeatureData is missing",
		"Missing game content — run 'ludus init --fix' to overlay content from Epic Launcher"},
	{"C1076: compiler limit",
		"Out of memory during compilation — reduce parallel jobs with -j flag (e.g. -j 4)"},
	{"C3859: Failed to create virtual memory",
		"Out of memory during PCH compilation — reduce parallel jobs with -j flag"},
	{"GetLastError=4551",
		"Windows Smart App Control (SAC) blocked a DLL. SAC blocks all unsigned binaries, including UE5 code compiled from source. " +
			"Turn off SAC (this does NOT disable Windows Defender antivirus): " +
			"Windows Security > App & browser control > Smart App Control > Off. " +
			"Run 'ludus init' for details. " +
			"See: https://support.microsoft.com/en-us/topic/what-is-smart-app-control-285ea03d-fa88-4d56-882e-6698afdb7003"},
	{"NU1903",
		"NuGet security audit failure — this should be handled automatically; please report as a bug"},
	{"error C4756:",
		"Overflow in constant arithmetic (MSVC C4756) — run 'ludus init --fix' to patch affected source files"},
	{"code integrity policy",
		"Windows Smart App Control (SAC) blocked execution — run 'ludus init' to see Microsoft's recommended options"},
	{"LINUX_MULTIARCH_ROOT",
		"Linux cross-compile toolchain not found — run 'ludus init --fix' to install, then restart your terminal"},
	{"AddBuildProductsFromManifest",
		"A build product listed in the UBT manifest was not generated. " +
			"For .sym files on ARM64 cross-compile, this is caused by dump_syms.exe crashing — " +
			"ludus should handle this automatically; please report as a bug if it persists"},
}

// scanBuildLogs reads the RunUAT log file and returns hints for any known
// error patterns found. Returns nil if the log doesn't exist or no patterns match.
func scanBuildLogs(enginePath string) []string {
	if enginePath == "" {
		return nil
	}

	logPath := filepath.Join(enginePath, "Engine", "Programs",
		"AutomationTool", "Saved", "Logs", "Log.txt")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}

	content := string(data)
	var hints []string
	seen := make(map[string]bool)
	for _, p := range knownLogPatterns {
		if strings.Contains(content, p.pattern) && !seen[p.hint] {
			hints = append(hints, p.hint)
			seen[p.hint] = true
		}
	}
	return hints
}

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

	serverTarget := b.opts.ServerTarget
	if serverTarget == "" {
		serverTarget = b.opts.ProjectName + "Server"
	}
	gameTarget := b.opts.GameTarget
	if gameTarget == "" {
		gameTarget = b.opts.ProjectName + "Game"
	}

	// Insert DefaultServerTarget after DefaultGameTarget in the BuildSettings section
	old := "DefaultGameTarget=" + gameTarget
	replacement := old + "\nDefaultServerTarget=" + serverTarget

	if !strings.Contains(content, old) {
		// If the expected DefaultGameTarget line is not found, skip gracefully
		fmt.Printf("  %s does not contain DefaultGameTarget=%s, skipping DefaultServerTarget configuration\n", iniPath, gameTarget)
		return nil
	}

	content = strings.Replace(content, old, replacement, 1)
	fmt.Printf("  Setting DefaultServerTarget=%s in %s\n", serverTarget, iniPath)
	return os.WriteFile(iniPath, []byte(content), 0644)
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

	content := string(data)
	if strings.Contains(content, "TargetArchitecture=AArch64") {
		return nil
	}

	section := "[/Script/LinuxTargetPlatform.LinuxTargetSettings]"
	entry := "TargetArchitecture=AArch64"

	if strings.Contains(content, section) {
		// Append the setting after the section header
		content = strings.Replace(content, section, section+"\n"+entry, 1)
	} else {
		// Append the entire section at the end
		content += "\n" + section + "\n" + entry + "\n"
	}

	fmt.Printf("  Setting %s in %s\n", entry, iniPath)
	return os.WriteFile(iniPath, []byte(content), 0644)
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

	content := string(original)
	tag := "<bDisableDumpSyms>true</bDisableDumpSyms>"

	if strings.Contains(content, tag) {
		return func() {} // already set
	}

	// Insert into existing <BuildConfiguration> section, or add one before </Configuration>.
	switch {
	case strings.Contains(content, "<BuildConfiguration>"):
		content = strings.Replace(content, "<BuildConfiguration>",
			"<BuildConfiguration>\n    "+tag, 1)
	case strings.Contains(content, "</Configuration>"):
		content = strings.Replace(content, "</Configuration>",
			"  <BuildConfiguration>\n    "+tag+"\n  </BuildConfiguration>\n</Configuration>", 1)
	default:
		fmt.Printf("  Warning: unrecognized BuildConfiguration.xml format, cannot disable dump_syms\n")
		return func() {}
	}

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		fmt.Printf("  Warning: could not update %s: %v\n", configPath, err)
		return func() {}
	}

	fmt.Println("  Disabled dump_syms in BuildConfiguration.xml (ARM64 cross-compile workaround)")

	return func() {
		if err := os.WriteFile(configPath, original, 0644); err != nil {
			fmt.Printf("  Warning: could not restore %s: %v\n", configPath, err)
		}
	}
}
