package dockerbuild

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/internal/ecr"
	"github.com/devrecon/ludus/internal/runner"
)

// EngineImageOptions configures the engine container image build.
type EngineImageOptions struct {
	// SourcePath is the path to the Unreal Engine source directory.
	SourcePath string
	// Version is the engine version tag (e.g. "5.6.1").
	Version string
	// MaxJobs limits parallel compile jobs inside the container.
	MaxJobs int
	// ImageName is the local image name (default: "ludus-engine").
	ImageName string
	// ImageTag is the image tag (default: engine version or "latest").
	ImageTag string
	// NoCache disables build cache.
	NoCache bool
	// BaseImage is the base image (e.g. "ubuntu:22.04", "amazonlinux:2023").
	BaseImage string
	// Runtime is the container backend: "docker" or "podman".
	Runtime string
	// SkipCompile skips engine compilation and packages pre-built Linux
	// binaries from the source tree into the image.
	SkipCompile bool
}

// EngineImageResult holds the outcome of an engine Docker image build.
type EngineImageResult struct {
	// ImageTag is the full image:tag string (e.g. "ludus-engine:5.6.1").
	ImageTag string
	// Duration is the build time in seconds.
	Duration float64
}

// EngineImageBuilder builds UE5 engine Docker images.
type EngineImageBuilder struct {
	opts   EngineImageOptions
	Runner *runner.Runner
}

// NewEngineImageBuilder creates a new engine image builder.
func NewEngineImageBuilder(opts EngineImageOptions, r *runner.Runner) *EngineImageBuilder {
	if opts.ImageName == "" {
		opts.ImageName = "ludus-engine"
	}
	if opts.ImageTag == "" {
		if opts.Version != "" {
			opts.ImageTag = opts.Version
		} else {
			opts.ImageTag = "latest"
		}
	}
	return &EngineImageBuilder{opts: opts, Runner: r}
}

// FullImageTag returns the image:tag string.
func (b *EngineImageBuilder) FullImageTag() string {
	return fmt.Sprintf("%s:%s", b.opts.ImageName, b.opts.ImageTag)
}

// Build creates a container image containing the built UE5 engine.
// The Dockerfile is written to a temp file; the engine source directory is the build context.
func (b *EngineImageBuilder) Build(ctx context.Context) (*EngineImageResult, error) {
	start := time.Now()

	if b.opts.SourcePath == "" {
		return nil, fmt.Errorf("engine source path not specified")
	}

	cli := ContainerCLI(b.opts.Runtime)

	// Generate Dockerfile and .dockerignore in a temp directory
	tmpDir, err := os.MkdirTemp("", "ludus-engine-docker-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dfOpts := DockerfileOptions{
		MaxJobs:   b.opts.MaxJobs,
		BaseImage: b.opts.BaseImage,
	}

	var dockerfile, dockerignore string
	if b.opts.SkipCompile {
		dockerfile = GeneratePrebuiltEngineDockerfile(dfOpts)
		dockerignore = GeneratePrebuiltEngineDockerignore()
	} else {
		dockerfile = GenerateEngineDockerfile(dfOpts)
		dockerignore = GenerateEngineDockerignore()
	}

	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
		return nil, fmt.Errorf("writing Dockerfile: %w", err)
	}

	// Write .dockerignore into the engine source dir (build context)
	dockerignorePath := filepath.Join(b.opts.SourcePath, ".dockerignore")
	if err := os.WriteFile(dockerignorePath, []byte(dockerignore), 0644); err != nil {
		return nil, fmt.Errorf("writing .dockerignore: %w", err)
	}
	defer os.Remove(dockerignorePath)

	// Build the image
	imageTag := b.FullImageTag()
	args := []string{
		"build",
		"--build-arg", fmt.Sprintf("MAX_JOBS=%d", b.opts.MaxJobs),
		"-t", imageTag,
		"-f", dockerfilePath,
	}
	if b.opts.NoCache {
		args = append(args, "--no-cache")
	}
	args = append(args, b.opts.SourcePath)

	if err := b.Runner.Run(ctx, cli, args...); err != nil {
		return nil, fmt.Errorf("%s build failed: %w", cli, err)
	}

	return &EngineImageResult{
		ImageTag: imageTag,
		Duration: time.Since(start).Seconds(),
	}, nil
}

// Push authenticates with ECR, tags the engine image, and pushes it.
// Creates the ECR repository if it does not already exist.
func (b *EngineImageBuilder) Push(ctx context.Context, opts ecr.PushOptions) error {
	if opts.ECRRepository == "" {
		opts.ECRRepository = b.opts.ImageName
	}
	if opts.ImageTag == "" {
		opts.ImageTag = b.opts.ImageTag
	}
	localTag := b.FullImageTag()
	if err := ecr.Push(ctx, b.Runner, localTag, opts); err != nil {
		return err
	}
	remoteTag := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
		opts.AWSAccountID, opts.AWSRegion, opts.ECRRepository, opts.ImageTag)
	fmt.Printf("  Pushed engine image: %s\n", remoteTag)
	return nil
}
