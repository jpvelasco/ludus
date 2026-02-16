package lyra

import "context"

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
	opts BuildOptions
}

// NewBuilder creates a new Lyra builder.
func NewBuilder(opts BuildOptions) *Builder {
	return &Builder{opts: opts}
}

// Build runs the full BuildCookRun pipeline for the Lyra server.
func (b *Builder) Build(ctx context.Context) (*BuildResult, error) {
	// TODO: Implement
	// 1. Locate RunUAT.sh in the engine directory
	// 2. Run BuildCookRun with:
	//    -project=<LyraPath>
	//    -platform=Linux
	//    -server -noclient
	//    -cook -build -stage -package -archive
	// 3. Stream output and capture result
	return &BuildResult{}, nil
}

// LocateProject finds the Lyra project within the engine source tree.
func (b *Builder) LocateProject() (string, error) {
	// TODO: Implement
	// Check: <EnginePath>/Samples/Games/Lyra/Lyra.uproject
	return "", nil
}
