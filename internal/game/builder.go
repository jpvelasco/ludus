package game

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/progress"
	"github.com/jpvelasco/ludus/internal/runner"
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

	if config.NormalizeArch(b.opts.Arch) == "arm64" {
		defer b.disableDumpSyms()()
	}
	return nil
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
