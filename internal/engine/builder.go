package engine

import "context"

// BuildOptions configures the engine build.
type BuildOptions struct {
	// SourcePath is the path to the UE source directory.
	SourcePath string
	// MaxJobs limits parallel compile jobs.
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
	opts BuildOptions
}

// NewBuilder creates a new engine builder.
func NewBuilder(opts BuildOptions) *Builder {
	return &Builder{opts: opts}
}

// Setup runs Setup.sh to download engine dependencies.
func (b *Builder) Setup(ctx context.Context) error {
	// TODO: Implement
	// 1. Validate SourcePath exists and contains Setup.sh
	// 2. Run Setup.sh with output streaming
	// 3. Handle errors and retry logic
	return nil
}

// GenerateProjectFiles runs GenerateProjectFiles.sh.
func (b *Builder) GenerateProjectFiles(ctx context.Context) error {
	// TODO: Implement
	return nil
}

// Build compiles the engine.
func (b *Builder) Build(ctx context.Context) (*BuildResult, error) {
	// TODO: Implement
	// 1. Run Setup if not already done
	// 2. Generate project files
	// 3. Run make with configured parallelism
	// 4. Build both Editor and Server targets
	return &BuildResult{}, nil
}
