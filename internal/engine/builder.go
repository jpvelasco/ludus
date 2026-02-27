package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/internal/runner"
)

// BuildOptions configures the engine build.
type BuildOptions struct {
	// SourcePath is the path to the UE source directory.
	SourcePath string
	// MaxJobs limits parallel compile jobs. 0 = auto-detect based on RAM.
	MaxJobs int
	// Verbose enables detailed build output.
	Verbose bool
}

// BuildResult holds the outcome of an engine build.
type BuildResult struct {
	// Success indicates whether the build completed.
	Success bool
	// EnginePath is the path to the built engine.
	EnginePath string
	// Duration is the build time in seconds.
	Duration float64
	// Error is set if the build failed.
	Error error
}

// Builder handles Unreal Engine compilation from source.
type Builder struct {
	opts   BuildOptions
	Runner *runner.Runner
}

// NewBuilder creates a new engine builder.
func NewBuilder(opts BuildOptions, r *runner.Runner) *Builder {
	return &Builder{opts: opts, Runner: r}
}

// Build compiles the engine: Setup → GenerateProjectFiles → compile targets.
func (b *Builder) Build(ctx context.Context) (*BuildResult, error) {
	start := time.Now()
	result := &BuildResult{EnginePath: b.opts.SourcePath}

	// Step 1: Setup
	fmt.Println("  Running Setup...")
	if err := b.Setup(ctx); err != nil {
		result.Error = fmt.Errorf("setup failed: %w", err)
		return result, result.Error
	}

	// Step 2: Generate project files
	fmt.Println("  Generating project files...")
	if err := b.GenerateProjectFiles(ctx); err != nil {
		result.Error = fmt.Errorf("generate project files failed: %w", err)
		return result, result.Error
	}

	// Step 3: Compile targets
	jobs := b.opts.MaxJobs
	if jobs == 0 {
		jobs = autoDetectJobs()
	}
	fmt.Printf("  Compiling with %d parallel job(s)...\n", jobs)

	if err := b.compile(ctx, jobs); err != nil {
		result.Error = err
		return result, result.Error
	}

	result.Success = true
	result.Duration = time.Since(start).Seconds()
	return result, nil
}
