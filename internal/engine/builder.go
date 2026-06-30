package engine

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/jpvelasco/ludus/internal/progress"
	"github.com/jpvelasco/ludus/internal/runner"
)

// BuildOptions configures the engine build.
type BuildOptions struct {
	// SourcePath is the path to the UE source directory.
	SourcePath string
	// MaxJobs limits parallel compile jobs. 0 = auto-detect based on RAM.
	MaxJobs int
	// Verbose enables detailed build output.
	Verbose bool
	// SkipSetup bypasses the Setup step (Setup.sh / Setup.bat). On headless
	// Windows, Setup.bat's bundled redist installers (VC++, GameInput) block
	// waiting on UI that never appears, wedging the build. Set this when the
	// engine dependencies have already been fetched (e.g. a prior GitDependencies
	// run) so the build can proceed straight to GenerateProjectFiles + compile.
	SkipSetup bool
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

	// Step 1: Setup (skippable — see BuildOptions.SkipSetup)
	if b.opts.SkipSetup {
		fmt.Println("  Skipping Setup (--skip-setup); assuming dependencies are already present...")
	} else {
		fmt.Println("  Running Setup...")
		if err := b.Setup(ctx); err != nil {
			result.Error = fmt.Errorf("setup failed: %w", err)
			return result, result.Error
		}
	}

	// Step 2: Generate project files.
	// On Windows, Build.bat invokes UBT directly and does not need VS project
	// files, so a GenerateProjectFiles failure is non-fatal. On Linux, make
	// depends on the Makefiles it produces, so it remains required.
	fmt.Println("  Generating project files...")
	if err := b.GenerateProjectFiles(ctx); err != nil {
		if runtime.GOOS == "windows" {
			fmt.Printf("  Warning: %v (continuing — Build.bat does not need project files)\n", err)
		} else {
			result.Error = fmt.Errorf("generate project files failed: %w", err)
			return result, result.Error
		}
	}

	// Step 3: Compile targets
	jobs := b.opts.MaxJobs
	if jobs == 0 {
		jobs = autoDetectJobs()
	}
	fmt.Printf("  Compiling with %d parallel job(s)...\n", jobs)

	ticker := progress.Start("Engine compile", 2*time.Minute)
	compileErr := b.compile(ctx, jobs)
	ticker.Stop()
	if compileErr != nil {
		result.Error = compileErr
		return result, result.Error
	}

	result.Success = true
	result.Duration = time.Since(start).Seconds()
	return result, nil
}
