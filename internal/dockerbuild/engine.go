package dockerbuild

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/jpvelasco/ludus/internal/ecr"
	"github.com/jpvelasco/ludus/internal/runner"
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
	// SkipEngine skips engine compilation and packages pre-built Linux
	// binaries from the source tree into the image.
	SkipEngine bool
	// Arch is the target CPU architecture: "amd64" or "arm64".
	// Used to set --platform linux/<arch> on the build command.
	Arch string
}

// platformArg returns the --platform argument value for this build.
func (o EngineImageOptions) platformArg() string {
	if o.Arch == "arm64" {
		return "linux/arm64"
	}
	return "linux/amd64"
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

	if b.opts.SkipEngine {
		if err := validateSkipEngine(b.opts.SourcePath); err != nil {
			return nil, err
		}
	}

	// macOS pre-flights: run Setup.sh and GenerateProjectFiles.sh inside Linux
	// containers so the host engine tree has the Linux toolchain and Makefile.
	if runtime.GOOS == "darwin" && !b.opts.SkipEngine {
		pfOpts := MacOSPreflightOptions{
			EngineSourcePath: b.opts.SourcePath,
			EngineVersion:    b.opts.Version,
			BaseImage:        b.opts.BaseImage,
			Runtime:          b.opts.Runtime,
			Arch:             "amd64", // force amd64 (Epic x86_64 toolchain only; arm64 game is cross inside)
		}
		if err := RunLinuxToolchainBootstrap(ctx, pfOpts, b.Runner); err != nil {
			return nil, fmt.Errorf("macOS Linux toolchain bootstrap: %w", err)
		}
		if err := RunLinuxGenerateProjectFiles(ctx, pfOpts, b.Runner); err != nil {
			return nil, fmt.Errorf("macOS GenerateProjectFiles: %w", err)
		}
	}

	tmpDir, err := os.MkdirTemp("", "ludus-engine-docker-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dfPath, cleanupIgnore, err := b.writeBuildContext(tmpDir)
	if err != nil {
		return nil, err
	}
	defer cleanupIgnore()

	imageTag := b.FullImageTag()
	// Force --platform linux/amd64 (Epic toolchain is x86_64 only; arm64 output via cross at game layer).
	args := []string{
		"build",
		"--platform", "linux/amd64",
		"--build-arg", fmt.Sprintf("MAX_JOBS=%d", b.opts.MaxJobs),
		"-t", imageTag,
		"-f", dfPath,
	}
	if b.opts.NoCache {
		args = append(args, "--no-cache")
	}
	args = append(args, b.opts.SourcePath)

	if err := b.Runner.Run(ctx, cli, args...); err != nil {
		return nil, wrapBuildError(cli, err)
	}

	return &EngineImageResult{
		ImageTag: imageTag,
		Duration: time.Since(start).Seconds(),
	}, nil
}

// validateSkipEngine checks that pre-built Linux binaries exist for skip-engine mode.
func validateSkipEngine(sourcePath string) error {
	binDir := filepath.Join(sourcePath, "Engine", "Binaries", "Linux")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("--skip-engine requires pre-built Linux binaries at %s; "+
				"run a native engine build first: ludus engine build", binDir)
		}
		return fmt.Errorf("reading pre-built binaries directory %s: %w", binDir, err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("--skip-engine found empty %s; "+
			"run a native engine build first: ludus engine build", binDir)
	}
	return nil
}

// writeBuildContext generates the Dockerfile and .dockerignore, writes them to
// disk, and returns the Dockerfile path plus a cleanup func for the .dockerignore.
func (b *EngineImageBuilder) writeBuildContext(tmpDir string) (string, func(), error) {
	dfOpts := DockerfileOptions{MaxJobs: b.opts.MaxJobs, BaseImage: b.opts.BaseImage, MacOSHost: runtime.GOOS == "darwin"}
	var dockerfile, dockerignore string
	if b.opts.SkipEngine {
		dockerfile = GeneratePrebuiltEngineDockerfile(dfOpts)
		dockerignore = GeneratePrebuiltEngineDockerignore()
	} else {
		dockerfile = GenerateEngineDockerfile(dfOpts)
		dockerignore = GenerateEngineDockerignore()
	}
	dfPath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte(dockerfile), 0644); err != nil {
		return "", nil, fmt.Errorf("writing Dockerfile: %w", err)
	}
	ignorePath := filepath.Join(b.opts.SourcePath, ".dockerignore")
	if err := os.WriteFile(ignorePath, []byte(dockerignore), 0644); err != nil {
		return "", nil, fmt.Errorf("writing .dockerignore: %w", err)
	}
	return dfPath, func() { os.Remove(ignorePath) }, nil
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
