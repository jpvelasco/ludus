package lyra

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

	runatPath := filepath.Join(b.opts.EnginePath, "Engine", "Build", "BatchFiles", "RunUAT.sh")
	if _, err := os.Stat(runatPath); os.IsNotExist(err) {
		result.Error = fmt.Errorf("RunUAT.sh not found at %s", runatPath)
		return result, result.Error
	}

	outputDir := b.opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(filepath.Dir(projectPath), "PackagedServer")
	}
	result.OutputDir = outputDir

	args := []string{
		runatPath,
		"BuildCookRun",
		"-project=" + projectPath,
		"-platform=Linux",
		"-server",
		"-noclient",
		"-build",
		"-stage",
		"-package",
		"-archive",
		"-archivedirectory=" + outputDir,
	}

	if !b.opts.SkipCook {
		args = append(args, "-cook")
	} else {
		args = append(args, "-skipcook")
	}

	if b.opts.ServerMap != "" {
		args = append(args, "-map="+b.opts.ServerMap)
	}

	if err := b.Runner.RunInDir(ctx, b.opts.EnginePath, "bash", args...); err != nil {
		result.Error = fmt.Errorf("BuildCookRun failed: %w", err)
		return result, result.Error
	}

	result.Success = true
	result.ServerBinary = filepath.Join(outputDir, "LinuxServer", "LyraServer")
	result.Duration = time.Since(start).Seconds()
	return result, nil
}
