package container

import "context"

// BuildOptions configures the container image build.
type BuildOptions struct {
	// ServerBuildDir is the path to the packaged Lyra server.
	ServerBuildDir string
	// ImageName is the Docker image name.
	ImageName string
	// Tag is the Docker image tag.
	Tag string
	// ServerPort is the game server port.
	ServerPort int
	// NoCache disables Docker build cache.
	NoCache bool
}

// BuildResult holds the outcome of a container build.
type BuildResult struct {
	// Success indicates whether the build completed.
	Success bool
	// ImageID is the built Docker image ID.
	ImageID string
	// ImageTag is the full image:tag string.
	ImageTag string
	// Error is set if the build failed.
	Error error
}

// Builder handles Docker container image creation.
type Builder struct {
	opts BuildOptions
}

// NewBuilder creates a new container builder.
func NewBuilder(opts BuildOptions) *Builder {
	return &Builder{opts: opts}
}

// Build creates the Docker image for the Lyra dedicated server.
func (b *Builder) Build(ctx context.Context) (*BuildResult, error) {
	// TODO: Implement
	// 1. Generate Dockerfile from embedded template
	// 2. Copy server build into Docker context
	// 3. Build Go SDK wrapper (or copy pre-built binary)
	// 4. Generate wrapper.sh with correct port and binary path
	// 5. Run docker build
	return &BuildResult{}, nil
}

// GenerateDockerfile creates a Dockerfile for the Lyra server container.
func (b *Builder) GenerateDockerfile() (string, error) {
	// TODO: Implement
	// Template based on GameLift Containers Starter Kit:
	// - Base: public.ecr.aws/amazonlinux/amazonlinux:latest
	// - Install Go for SDK wrapper
	// - Create non-root user (required for Unreal)
	// - Copy server build and wrapper
	// - Set entrypoint to wrapper.sh
	return "", nil
}
