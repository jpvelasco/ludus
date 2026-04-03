package game

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/devrecon/ludus/internal/progress"
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

	restoreDDC, err := b.applyDDCConfig(projectPath)
	if err != nil {
		result.Error = err
		return result, err
	}
	defer restoreDDC()

	outputDir := b.opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(filepath.Dir(projectPath), "PackagedClient")
	}
	result.OutputDir = outputDir

	args := b.clientBuildArgs(projectPath, platform, outputDir)

	ticker := progress.Start("Client build", 2*time.Minute)
	buildErr := b.execRunUAT(ctx, shell, runatPath, args)
	ticker.Stop()
	if buildErr != nil {
		result.Error = diagnoseBuildError(buildErr, fmt.Sprintf("BuildCookRun (client, %s)", platform), b.opts.EnginePath)
		return result, result.Error
	}

	result.Success = true
	result.ClientBinary = b.clientBinaryPath(outputDir, platform)
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
	if !b.opts.SkipCook {
		args = append(args, "-cook")
	} else {
		args = append(args, "-skipcook")
	}
	isCrossCompile := runtime.GOOS == "windows" && platform == "Linux"
	if jobs := b.resolveMaxJobs(isCrossCompile); jobs > 0 {
		args = append(args, fmt.Sprintf("-MaxParallelActions=%d", jobs))
		fmt.Printf("  Limiting parallel compile actions to %d\n", jobs)
	}
	return args
}

// clientBinaryPath returns the expected client binary path for the given platform.
func (b *Builder) clientBinaryPath(outputDir, platform string) string {
	projectName := b.opts.ProjectName
	if projectName == "" {
		projectName = "Lyra"
	}
	clientTarget := b.opts.ClientTarget
	if clientTarget == "" {
		clientTarget = projectName + "Game"
	}

	switch platform {
	case "Win64":
		return filepath.Join(outputDir, "Windows", projectName, "Binaries", "Win64", clientTarget+".exe")
	default:
		return filepath.Join(outputDir, "Linux", projectName, "Binaries", "Linux", clientTarget)
	}
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
	clientBin := b.clientBinaryPath(outputDir, platform)
	if _, err := os.Stat(clientBin); err == nil {
		return "" // full build already exists
	}

	return fmt.Sprintf("Previous cooked content found at %s\n"+
		"  To skip re-cooking, re-run with: ludus game client --skip-cook", cookedDir)
}
