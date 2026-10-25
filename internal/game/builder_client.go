package game

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/progress"
)

// ClientBuildResult holds the outcome of a game client build.
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

// BuildClient runs the BuildCookRun pipeline for the standalone game client.
// Supports building for Linux (native) or Win64 (cross-compile, requires toolchain).
func (b *Builder) BuildClient(ctx context.Context) (*ClientBuildResult, error) {
	start := time.Now()
	result := &ClientBuildResult{}

	platform, err := b.resolveClientPlatform()
	if err != nil {
		result.Error = err
		return result, err
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

	b.applyNuGetAuditWorkaround()
	b.ensureLinuxMultiarchRoot()

	outputDir := b.opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(filepath.Dir(projectPath), "PackagedClient")
	}
	result.OutputDir = outputDir

	if err := b.setupDDC(); err != nil {
		result.Error = err
		return result, err
	}
	args := b.clientBuildArgs(projectPath, platform, outputDir)

	ticker := progress.Start("Client build", 2*time.Minute)
	buildErr := b.execRunUAT(ctx, shell, runatPath, args)
	ticker.Stop()
	if buildErr != nil {
		result.Error = diagnoseBuildError(buildErr, fmt.Sprintf("BuildCookRun (client, %s)", platform), b.opts.EnginePath)
		return result, result.Error
	}

	result.Success = true
	result.ClientBinary = b.clientBinaryPath(projectPath, outputDir, platform)
	result.Duration = time.Since(start).Seconds()
	return result, nil
}

func (b *Builder) resolveClientPlatform() (string, error) {
	platform := b.opts.ClientPlatform
	if platform == "" {
		platform = "Linux"
	}
	switch platform {
	case "Linux", "Win64":
		return platform, nil
	default:
		return "", fmt.Errorf("unsupported client platform %q (supported: Linux, Win64)", platform)
	}
}

func (b *Builder) clientBuildArgs(projectPath, platform, outputDir string) []string {
	args := []string{
		"BuildCookRun",
		fmt.Sprintf(`-project="%s"`, projectPath),
		"-platform=" + platform,
		"-build", "-stage", "-package", "-archive",
		fmt.Sprintf(`-archivedirectory="%s"`, outputDir),
	}
	args = append(args, b.clientTargetArgs()...)
	args = append(args, b.cookArgs()...)
	isCrossCompile := runtime.GOOS == "windows" && platform == "Linux"
	if jobs := b.resolveMaxJobs(isCrossCompile); jobs > 0 {
		args = append(args, fmt.Sprintf("-MaxParallelActions=%d", jobs))
		fmt.Printf("  Limiting parallel compile actions to %d\n", jobs)
	}
	return args
}

// clientTargetArgs selects the game (client) target for BuildCookRun. It
// mirrors how the server build passes its target: -target is UAT's game target
// selector, so multi-target projects build the configured client target
// instead of the project default. An empty ClientTarget defers to UAT's own
// target resolution (#740).
func (b *Builder) clientTargetArgs() []string {
	if b.opts.ClientTarget == "" {
		return nil
	}
	return []string{fmt.Sprintf("-target=%s", b.opts.ClientTarget)}
}

// clientBinaryPath returns the path to the staged client binary for the given
// platform. It discovers the actual executable inside the packaged project
// directory — UE names that directory after the .uproject filename, which may
// differ from ProjectName (e.g. ExampleContent.uproject with projectName=
// Example), and names the binary after the project's real client target, which
// need not match ProjectName+"Game" (e.g. Lyra's LyraStarterGame.uproject
// builds LyraGame) — falling back to the conventional name when discovery is
// not possible (e.g. a dry run).
func (b *Builder) clientBinaryPath(projectPath, outputDir, platform string) string {
	projectName := b.opts.ProjectName
	if projectName == "" {
		projectName = "Lyra"
	}
	clientTarget := b.opts.ClientTarget
	if clientTarget == "" {
		clientTarget = projectName + "Game"
	}

	isWindows := platform == "Win64"
	platformDir := "Linux"
	binSub := "Linux"
	suffix := ""
	if isWindows {
		platformDir, binSub, suffix = "Windows", "Win64", ".exe"
	}

	binariesDir := filepath.Join(outputDir, platformDir, b.packagedDirName(projectPath), "Binaries", binSub)
	fallback := filepath.Join(binariesDir, clientTarget+suffix)
	return config.DiscoverClientBinary(binariesDir, fallback, isWindows)
}

// PartialClientBuildHint checks for cooked content from a previous client build.
// Returns empty string if no partial build is detected or --skip-cook is set.
func (b *Builder) PartialClientBuildHint() string {
	if b.opts.SkipCook {
		return ""
	}

	projectPath, err := b.LocateProject()
	if err != nil {
		return ""
	}

	projectDir := filepath.Dir(projectPath)
	platform := b.opts.ClientPlatform
	if platform == "" {
		platform = "Linux"
	}

	// Map platform name to UE cooked directory name
	cookedPlatform := platform
	if platform == "Win64" {
		cookedPlatform = "Windows"
	}
	cookedDir := filepath.Join(projectDir, "Saved", "Cooked", cookedPlatform)
	if !dirHasContent(cookedDir) {
		return ""
	}

	// Check if the final client output is complete
	outputDir := b.opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(projectDir, "PackagedClient")
	}
	clientBin := b.clientBinaryPath(projectPath, outputDir, platform)
	if _, err := os.Stat(clientBin); err == nil {
		return "" // full build already exists
	}

	return fmt.Sprintf("Previous cooked content found at %s\n"+
		"  To skip re-cooking, re-run with: ludus game client --skip-cook", cookedDir)
}
