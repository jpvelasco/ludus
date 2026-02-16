package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// Setup runs Setup.sh to download engine dependencies.
func (b *Builder) Setup(ctx context.Context) error {
	setupPath := filepath.Join(b.opts.SourcePath, "Setup.sh")
	if _, err := os.Stat(setupPath); os.IsNotExist(err) {
		return fmt.Errorf("Setup.sh not found at %s", setupPath)
	}

	return b.Runner.RunInDir(ctx, b.opts.SourcePath, "bash", "Setup.sh")
}

// GenerateProjectFiles runs GenerateProjectFiles.sh.
func (b *Builder) GenerateProjectFiles(ctx context.Context) error {
	genPath := filepath.Join(b.opts.SourcePath, "GenerateProjectFiles.sh")
	if _, err := os.Stat(genPath); os.IsNotExist(err) {
		return fmt.Errorf("GenerateProjectFiles.sh not found at %s", genPath)
	}

	return b.Runner.RunInDir(ctx, b.opts.SourcePath, "bash", "GenerateProjectFiles.sh")
}

// autoDetectJobs calculates the number of parallel compile jobs based on
// available RAM. UE5 linking can spike ~8GB per job.
func autoDetectJobs() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 1
	}
	defer f.Close()

	var memKB uint64
	fmt.Fscanf(f, "MemTotal: %d kB", &memKB)
	if memKB == 0 {
		return 1
	}

	memGB := memKB / (1024 * 1024)
	jobs := max(int(memGB/8), 1)
	return jobs
}

// Build compiles the engine: Setup → GenerateProjectFiles → make.
func (b *Builder) Build(ctx context.Context) (*BuildResult, error) {
	start := time.Now()
	result := &BuildResult{EnginePath: b.opts.SourcePath}

	// Step 1: Setup
	if err := b.Setup(ctx); err != nil {
		result.Error = fmt.Errorf("setup failed: %w", err)
		return result, result.Error
	}

	// Step 2: Generate project files
	if err := b.GenerateProjectFiles(ctx); err != nil {
		result.Error = fmt.Errorf("generate project files failed: %w", err)
		return result, result.Error
	}

	// Step 3: Compile with make
	jobs := b.opts.MaxJobs
	if jobs == 0 {
		jobs = autoDetectJobs()
	}

	jobsFlag := fmt.Sprintf("-j%d", jobs)
	targets := []string{"ShaderCompileWorker", "UnrealEditor", "LyraServer"}

	for _, target := range targets {
		if err := b.Runner.RunInDir(ctx, b.opts.SourcePath, "make", jobsFlag, target); err != nil {
			result.Error = fmt.Errorf("make %s failed: %w", target, err)
			return result, result.Error
		}
	}

	result.Success = true
	result.Duration = time.Since(start).Seconds()
	return result, nil
}
