package container

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/internal/runner"
)

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

// PushOptions configures the ECR push.
type PushOptions struct {
	// ECRRepository is the ECR repository name.
	ECRRepository string
	// AWSRegion is the AWS region.
	AWSRegion string
	// AWSAccountID is the AWS account ID.
	AWSAccountID string
	// ImageTag is the image tag to push.
	ImageTag string
}

// BuildResult holds the outcome of a container build.
type BuildResult struct {
	// Success indicates whether the build completed.
	Success bool
	// ImageID is the built Docker image ID.
	ImageID string
	// ImageTag is the full image:tag string.
	ImageTag string
	// Duration is the build time in seconds.
	Duration float64
	// Error is set if the build failed.
	Error error
}

// Builder handles Docker container image creation.
type Builder struct {
	opts   BuildOptions
	Runner *runner.Runner
}

// NewBuilder creates a new container builder.
func NewBuilder(opts BuildOptions, r *runner.Runner) *Builder {
	return &Builder{opts: opts, Runner: r}
}

// GenerateDockerfile creates a Dockerfile for the Lyra server container.
// Based on the GameLift Containers Starter Kit pattern.
func (b *Builder) GenerateDockerfile() string {
	return fmt.Sprintf(`FROM public.ecr.aws/amazonlinux/amazonlinux:2023

# Install required runtime libraries
RUN dnf install -y \
    libicu \
    libnsl2 \
    libstdc++ \
    && dnf clean all

# Create non-root user (required for Unreal servers)
RUN useradd -m -s /bin/bash ueserver

# Create server directory
RUN mkdir -p /opt/server && chown ueserver:ueserver /opt/server

# Copy server build
COPY --chown=ueserver:ueserver . /opt/server/

# Make server binary executable
RUN chmod +x /opt/server/LyraServer

# Expose game server port
EXPOSE %d/udp

# Switch to non-root user
USER ueserver
WORKDIR /opt/server

# Entrypoint runs the Lyra dedicated server
ENTRYPOINT ["./LyraServer", "-port=%d", "-log"]
`, b.opts.ServerPort, b.opts.ServerPort)
}

// Build creates the Docker image for the Lyra dedicated server.
func (b *Builder) Build(ctx context.Context) (*BuildResult, error) {
	start := time.Now()
	result := &BuildResult{}

	if b.opts.ServerBuildDir == "" {
		result.Error = fmt.Errorf("server build directory not specified")
		return result, result.Error
	}

	// Generate Dockerfile in the server build directory
	dockerfilePath := filepath.Join(b.opts.ServerBuildDir, "Dockerfile")
	dockerfile := b.GenerateDockerfile()
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
		result.Error = fmt.Errorf("writing Dockerfile: %w", err)
		return result, result.Error
	}
	defer os.Remove(dockerfilePath)

	imageTag := fmt.Sprintf("%s:%s", b.opts.ImageName, b.opts.Tag)
	result.ImageTag = imageTag

	args := []string{"build", "-t", imageTag}
	if b.opts.NoCache {
		args = append(args, "--no-cache")
	}
	args = append(args, b.opts.ServerBuildDir)

	if err := b.Runner.Run(ctx, "docker", args...); err != nil {
		result.Error = fmt.Errorf("docker build failed: %w", err)
		return result, result.Error
	}

	result.Success = true
	result.Duration = time.Since(start).Seconds()
	return result, nil
}

// Push authenticates with ECR, tags the image, and pushes it.
func (b *Builder) Push(ctx context.Context, opts PushOptions) error {
	if opts.AWSAccountID == "" {
		return fmt.Errorf("AWS account ID not configured (set aws.accountId in ludus.yaml)")
	}

	ecrURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s",
		opts.AWSAccountID, opts.AWSRegion, opts.ECRRepository)

	// Authenticate with ECR
	loginURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", opts.AWSAccountID, opts.AWSRegion)
	if err := b.Runner.Run(ctx, "bash", "-c",
		fmt.Sprintf("aws ecr get-login-password --region %s | docker login --username AWS --password-stdin %s",
			opts.AWSRegion, loginURI)); err != nil {
		return fmt.Errorf("ECR login failed: %w", err)
	}

	// Tag for ECR
	localTag := fmt.Sprintf("%s:%s", b.opts.ImageName, b.opts.Tag)
	remoteTag := fmt.Sprintf("%s:%s", ecrURI, opts.ImageTag)
	if err := b.Runner.Run(ctx, "docker", "tag", localTag, remoteTag); err != nil {
		return fmt.Errorf("docker tag failed: %w", err)
	}

	// Push to ECR
	if err := b.Runner.Run(ctx, "docker", "push", remoteTag); err != nil {
		return fmt.Errorf("docker push failed: %w", err)
	}

	return nil
}
