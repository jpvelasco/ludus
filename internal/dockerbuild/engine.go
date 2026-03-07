package dockerbuild

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"strings"

	"github.com/devrecon/ludus/internal/runner"
)

// EngineImageOptions configures the engine Docker image build.
type EngineImageOptions struct {
	// SourcePath is the path to the Unreal Engine source directory.
	SourcePath string
	// Version is the engine version tag (e.g. "5.6.1").
	Version string
	// MaxJobs limits parallel compile jobs inside the container.
	MaxJobs int
	// ImageName is the local Docker image name (default: "ludus-engine").
	ImageName string
	// ImageTag is the image tag (default: engine version or "latest").
	ImageTag string
	// NoCache disables Docker build cache.
	NoCache bool
	// BaseImage is the Docker base image (e.g. "ubuntu:22.04", "amazonlinux:2023").
	BaseImage string
}

// EngineImageResult holds the outcome of an engine Docker image build.
type EngineImageResult struct {
	// ImageTag is the full image:tag string (e.g. "ludus-engine:5.6.1").
	ImageTag string
	// Duration is the build time in seconds.
	Duration float64
}

// PushOptions configures the ECR push for the engine image.
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

// Build creates a Docker image containing the built UE5 engine.
// The Dockerfile is written to a temp file; the engine source directory is the build context.
func (b *EngineImageBuilder) Build(ctx context.Context) (*EngineImageResult, error) {
	start := time.Now()

	if b.opts.SourcePath == "" {
		return nil, fmt.Errorf("engine source path not specified")
	}

	// Generate Dockerfile and .dockerignore in a temp directory
	tmpDir, err := os.MkdirTemp("", "ludus-engine-docker-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dockerfile := GenerateEngineDockerfile(DockerfileOptions{
		MaxJobs:   b.opts.MaxJobs,
		BaseImage: b.opts.BaseImage,
	})
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
		return nil, fmt.Errorf("writing Dockerfile: %w", err)
	}

	// Write .dockerignore into the engine source dir (build context)
	dockerignorePath := filepath.Join(b.opts.SourcePath, ".dockerignore")
	if err := os.WriteFile(dockerignorePath, []byte(GenerateEngineDockerignore()), 0644); err != nil {
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

	if err := b.Runner.Run(ctx, "docker", args...); err != nil {
		return nil, fmt.Errorf("docker build failed: %w", err)
	}

	return &EngineImageResult{
		ImageTag: imageTag,
		Duration: time.Since(start).Seconds(),
	}, nil
}

// Push authenticates with ECR, tags the engine image, and pushes it.
// Creates the ECR repository if it does not already exist.
func (b *EngineImageBuilder) Push(ctx context.Context, opts PushOptions) error {
	if opts.AWSAccountID == "" {
		return fmt.Errorf("AWS account ID not configured (set aws.accountId in ludus.yaml)")
	}

	repoName := opts.ECRRepository
	if repoName == "" {
		repoName = b.opts.ImageName
	}

	ecrURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s",
		opts.AWSAccountID, opts.AWSRegion, repoName)

	// Ensure ECR repository exists
	if err := b.Runner.Run(ctx, "aws", "ecr", "describe-repositories",
		"--repository-names", repoName,
		"--region", opts.AWSRegion); err != nil {
		fmt.Printf("  ECR repository %q not found, creating...\n", repoName)
		if err := b.Runner.Run(ctx, "aws", "ecr", "create-repository",
			"--repository-name", repoName,
			"--region", opts.AWSRegion,
			"--image-scanning-configuration", "scanOnPush=true"); err != nil {
			return fmt.Errorf("creating ECR repository: %w", err)
		}
	}

	// Authenticate with ECR — get password then pipe to docker login (no shell)
	loginURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", opts.AWSAccountID, opts.AWSRegion)
	password, err := b.Runner.RunOutput(ctx, "aws", "ecr", "get-login-password", "--region", opts.AWSRegion)
	if err != nil {
		return fmt.Errorf("getting ECR password: %w", err)
	}
	if err := b.Runner.RunWithStdin(ctx, strings.NewReader(strings.TrimSpace(string(password))),
		"docker", "login", "--username", "AWS", "--password-stdin", loginURI); err != nil {
		return fmt.Errorf("ECR login failed: %w", err)
	}

	// Tag for ECR
	localTag := b.FullImageTag()
	tag := opts.ImageTag
	if tag == "" {
		tag = b.opts.ImageTag
	}
	remoteTag := fmt.Sprintf("%s:%s", ecrURI, tag)
	if err := b.Runner.Run(ctx, "docker", "tag", localTag, remoteTag); err != nil {
		return fmt.Errorf("docker tag failed: %w", err)
	}

	// Push to ECR
	if err := b.Runner.Run(ctx, "docker", "push", remoteTag); err != nil {
		return fmt.Errorf("docker push failed: %w", err)
	}

	fmt.Printf("  Pushed engine image: %s\n", remoteTag)
	return nil
}
