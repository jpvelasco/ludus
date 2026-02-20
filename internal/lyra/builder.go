package lyra

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/devrecon/ludus/internal/runner"
)

// BuildOptions configures the Lyra server build.
type BuildOptions struct {
	// EnginePath is the path to the built Unreal Engine.
	EnginePath string
	// ProjectPath is the path to the Lyra .uproject file.
	ProjectPath string
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
}

// BuildResult holds the outcome of a Lyra server build.
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

// Builder handles Lyra dedicated server compilation.
type Builder struct {
	opts   BuildOptions
	Runner *runner.Runner
}

// NewBuilder creates a new Lyra builder.
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

// LocateProject finds the Lyra project within the engine source tree.
func (b *Builder) LocateProject() (string, error) {
	if b.opts.ProjectPath != "" {
		if _, err := os.Stat(b.opts.ProjectPath); err != nil {
			return "", fmt.Errorf("configured project path not found: %s", b.opts.ProjectPath)
		}
		return b.opts.ProjectPath, nil
	}

	// Auto-detect from engine Samples directory
	candidate := filepath.Join(b.opts.EnginePath, "Samples", "Games", "Lyra", "Lyra.uproject")
	if _, err := os.Stat(candidate); err != nil {
		return "", fmt.Errorf("Lyra.uproject not found at %s (set lyra.projectPath in ludus.yaml)", candidate)
	}
	return candidate, nil
}

// Build runs the full BuildCookRun pipeline for the Lyra server.
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

	if err := b.ensureNuGetAuditDisabled(); err != nil {
		result.Error = fmt.Errorf("disabling NuGet audit: %w", err)
		return result, result.Error
	}

	if err := b.ensureDefaultServerTarget(projectPath); err != nil {
		result.Error = fmt.Errorf("setting default server target: %w", err)
		return result, result.Error
	}

	outputDir := b.opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(filepath.Dir(projectPath), "PackagedServer")
	}
	result.OutputDir = outputDir

	args := []string{
		"BuildCookRun",
		fmt.Sprintf(`-project="%s"`, projectPath),
		"-platform=Linux",
		"-server",
		"-noclient",
		"-servertargetname=LyraServer",
		"-build",
		"-stage",
		"-package",
		"-archive",
		fmt.Sprintf(`-archivedirectory="%s"`, outputDir),
	}

	if !b.opts.SkipCook {
		args = append(args, "-cook")
	} else {
		args = append(args, "-skipcook")
	}

	if b.opts.ServerMap != "" {
		args = append(args, fmt.Sprintf(`-map="%s"`, b.opts.ServerMap))
	}

	if err := b.execRunUAT(ctx, shell, runatPath, args); err != nil {
		result.Error = fmt.Errorf("BuildCookRun failed: %w", err)
		return result, result.Error
	}

	result.Success = true
	result.ServerBinary = filepath.Join(outputDir, "LinuxServer", "LyraServer")
	result.Duration = time.Since(start).Seconds()
	return result, nil
}

// ClientBuildResult holds the outcome of a Lyra client build.
type ClientBuildResult struct {
	// Success indicates whether the build completed.
	Success bool
	// OutputDir is the path to the packaged client build.
	OutputDir string
	// ClientBinary is the path to the client executable.
	ClientBinary string
	// Platform is the target platform the client was built for.
	Platform string
	// Duration is the build time in seconds.
	Duration float64
	// Error is set if the build failed.
	Error error
}

// BuildClient runs the BuildCookRun pipeline for the Lyra standalone game client.
// Supports building for Linux (native) or Win64 (cross-compile, requires toolchain).
func (b *Builder) BuildClient(ctx context.Context) (*ClientBuildResult, error) {
	start := time.Now()
	result := &ClientBuildResult{}

	platform := b.opts.ClientPlatform
	if platform == "" {
		platform = "Linux"
	}

	switch platform {
	case "Linux", "Win64":
		// supported
	default:
		result.Error = fmt.Errorf("unsupported client platform %q (supported: Linux, Win64)", platform)
		return result, result.Error
	}

	result.Platform = platform

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

	if err := b.ensureNuGetAuditDisabled(); err != nil {
		result.Error = fmt.Errorf("disabling NuGet audit: %w", err)
		return result, result.Error
	}

	outputDir := b.opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(filepath.Dir(projectPath), "PackagedClient")
	}
	result.OutputDir = outputDir

	args := []string{
		"BuildCookRun",
		fmt.Sprintf(`-project="%s"`, projectPath),
		"-platform=" + platform,
		"-build",
		"-stage",
		"-package",
		"-archive",
		fmt.Sprintf(`-archivedirectory="%s"`, outputDir),
	}

	if !b.opts.SkipCook {
		args = append(args, "-cook")
	} else {
		args = append(args, "-skipcook")
	}

	if err := b.execRunUAT(ctx, shell, runatPath, args); err != nil {
		result.Error = fmt.Errorf("BuildCookRun (client, %s) failed: %w", platform, err)
		return result, result.Error
	}

	result.Success = true
	result.ClientBinary = clientBinaryPath(outputDir, platform)
	result.Duration = time.Since(start).Seconds()
	return result, nil
}

// clientBinaryPath returns the expected client binary path for the given platform.
func clientBinaryPath(outputDir, platform string) string {
	switch platform {
	case "Win64":
		return filepath.Join(outputDir, "Windows", "Lyra", "Binaries", "Win64", "LyraGame.exe")
	default:
		return filepath.Join(outputDir, "Linux", "Lyra", "Binaries", "Linux", "LyraGame")
	}
}

// ensureNuGetAuditDisabled creates a Directory.Build.props in the engine's
// Programs directory to raise the NuGet audit severity threshold. UE 5.6's
// Gauntlet test framework directly depends on Magick.NET 14.7.0 which has
// known low/moderate/high CVEs. Combined with Epic's TreatWarningsAsErrors,
// this causes AutomationTool's script modules to fail to compile.
// Setting NuGetAuditLevel=critical still audits for critical vulnerabilities
// while allowing the non-critical Magick.NET CVEs through.
// Directory.Build.props is the standard MSBuild mechanism for this.
func (b *Builder) ensureNuGetAuditDisabled() error {
	propsPath := filepath.Join(b.opts.EnginePath, "Engine", "Source", "Programs", "Directory.Build.props")

	content := `<Project>
  <PropertyGroup>
    <!-- Only flag critical NuGet vulnerabilities as errors.
         UE 5.6's Gauntlet test framework directly depends on Magick.NET
         14.7.0 which has known low/moderate/high severity CVEs. Combined
         with Epic's TreatWarningsAsErrors, this causes AutomationTool
         script modules to fail to compile. Magick.NET is only used in
         Gauntlet's screenshot comparison for automated testing — it never
         ships in the Lyra server binary. Critical CVEs are still caught. -->
    <NuGetAuditLevel>critical</NuGetAuditLevel>
  </PropertyGroup>
</Project>
`

	existing, err := os.ReadFile(propsPath)
	if err == nil && string(existing) == content {
		return nil
	}

	fmt.Printf("  Writing %s to disable NuGet audit\n", propsPath)
	return os.WriteFile(propsPath, []byte(content), 0644)
}

// ensureDefaultServerTarget adds DefaultServerTarget=LyraServer to Lyra's
// DefaultEngine.ini if not already set. UE 5.6 Lyra ships with multiple
// server targets (LyraServer, LyraServerEOS, etc.) and RunUAT refuses to
// build without this setting, even when -servertargetname is passed on the
// command line.
func (b *Builder) ensureDefaultServerTarget(projectPath string) error {
	iniPath := filepath.Join(filepath.Dir(projectPath), "Config", "DefaultEngine.ini")

	data, err := os.ReadFile(iniPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", iniPath, err)
	}

	content := string(data)
	if strings.Contains(content, "DefaultServerTarget") {
		return nil
	}

	// Insert DefaultServerTarget after DefaultGameTarget in the BuildSettings section
	old := "DefaultGameTarget=LyraGame"
	replacement := old + "\nDefaultServerTarget=LyraServer"

	if !strings.Contains(content, old) {
		return fmt.Errorf("%s does not contain expected DefaultGameTarget=LyraGame", iniPath)
	}

	content = strings.Replace(content, old, replacement, 1)
	fmt.Printf("  Setting DefaultServerTarget=LyraServer in %s\n", iniPath)
	return os.WriteFile(iniPath, []byte(content), 0644)
}
