package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/internal/runner"
)

const (
	wrapperRepo    = "https://github.com/amazon-gamelift/amazon-gamelift-servers-game-server-wrapper.git"
	wrapperVersion = "v1.1.0"
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

// wrapperCacheDir returns the cache directory for the game server wrapper.
func wrapperCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "ludus", "game-server-wrapper"), nil
}

// ensureWrapper clones and builds the Amazon GameLift Game Server Wrapper,
// returning the path to the built binary. Results are cached in ~/.cache/ludus/.
func (b *Builder) ensureWrapper(ctx context.Context) (string, error) {
	cacheDir, err := wrapperCacheDir()
	if err != nil {
		return "", err
	}

	binaryPath := filepath.Join(cacheDir, "out", "linux", "amd64",
		"gamelift-servers-managed-containers", "amazon-gamelift-servers-game-server-wrapper")

	// Check if cached binary already exists
	if _, err := os.Stat(binaryPath); err == nil {
		fmt.Println("  Using cached game server wrapper binary")
		return binaryPath, nil
	}

	// Clone the repository
	fmt.Println("  Cloning game server wrapper repository...")
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0755); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}
	// Remove stale cache if it exists but binary is missing
	os.RemoveAll(cacheDir)

	if err := b.Runner.Run(ctx, "git", "clone", "--branch", wrapperVersion, "--depth", "1",
		wrapperRepo, cacheDir); err != nil {
		return "", fmt.Errorf("cloning game server wrapper: %w", err)
	}

	// Build the wrapper
	fmt.Println("  Building game server wrapper...")
	if err := b.Runner.RunInDir(ctx, cacheDir, "make", "build"); err != nil {
		// Clean up on build failure so next run retries
		os.RemoveAll(cacheDir)
		return "", fmt.Errorf("building game server wrapper: %w", err)
	}

	// Verify the binary was produced
	if _, err := os.Stat(binaryPath); err != nil {
		os.RemoveAll(cacheDir)
		return "", fmt.Errorf("wrapper binary not found after build at %s", binaryPath)
	}

	return binaryPath, nil
}

// GenerateWrapperConfig produces the config.yaml for the GameLift Game Server Wrapper.
// The wrapper uses this to know how to launch the Lyra server process.
func (b *Builder) GenerateWrapperConfig() string {
	return fmt.Sprintf(`log-config:
  wrapper-log-level: info

ports:
  gamePort: %d

game-server-details:
  executable-file-path: ./Lyra/Binaries/Linux/LyraServer
  game-server-args:
    - arg: "Lyra"
      val: ""
      pos: 0
    - arg: "-port="
      val: "{{.ContainerPort}}"
      pos: 1
    - arg: "-log"
      val: ""
      pos: 2
`, b.opts.ServerPort)
}

// copyFile copies a file from src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	// Preserve executable permission
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}

// GenerateDockerfile creates a Dockerfile for the Lyra server container.
// The packaged server from RunUAT BuildCookRun has this structure:
//
//	LinuxServer/
//	├── LyraServer.sh              (launcher script)
//	├── Engine/                     (engine runtime content)
//	└── Lyra/
//	    ├── Binaries/Linux/LyraServer  (actual server binary)
//	    ├── Config/
//	    ├── Content/
//	    └── Plugins/
//
// Based on the GameLift Containers Starter Kit pattern.
func (b *Builder) GenerateDockerfile() string {
	return fmt.Sprintf(`FROM public.ecr.aws/amazonlinux/amazonlinux:2023

# Install required runtime libraries
RUN dnf install -y \
    libicu \
    libnsl \
    libstdc++ \
    shadow-utils \
    && dnf clean all

# Create non-root user (required for Unreal servers)
RUN useradd -m -s /bin/bash ueserver

# Create server directory
RUN mkdir -p /opt/server && chown ueserver:ueserver /opt/server

# Copy packaged server, wrapper binary, and wrapper config
COPY --chown=ueserver:ueserver . /opt/server/

# Make binaries executable
RUN chmod +x /opt/server/amazon-gamelift-servers-game-server-wrapper \
    && chmod +x /opt/server/Lyra/Binaries/Linux/LyraServer

# Expose game server port
EXPOSE %d/udp

# Switch to non-root user
USER ueserver
WORKDIR /opt/server

# Wrapper is PID 1 — handles GameLift SDK, launches Lyra as child process
ENTRYPOINT ["./amazon-gamelift-servers-game-server-wrapper"]
`, b.opts.ServerPort)
}

// GenerateDockerignore creates a .dockerignore to exclude debug symbols
// and other files not needed at runtime. This saves ~1.7 GB.
func (b *Builder) GenerateDockerignore() string {
	return `# Debug symbols (saves ~1.7 GB)
**/*.debug
**/*.sym

# Build manifests
Manifest_*.txt
`
}

// Build creates the Docker image for the Lyra dedicated server.
func (b *Builder) Build(ctx context.Context) (*BuildResult, error) {
	start := time.Now()
	result := &BuildResult{}

	if b.opts.ServerBuildDir == "" {
		result.Error = fmt.Errorf("server build directory not specified")
		return result, result.Error
	}

	// 1. Build/fetch the GameLift Game Server Wrapper binary
	wrapperBin, err := b.ensureWrapper(ctx)
	if err != nil {
		result.Error = fmt.Errorf("game server wrapper: %w", err)
		return result, result.Error
	}

	// 2. Copy wrapper binary into server build directory
	wrapperDst := filepath.Join(b.opts.ServerBuildDir, "amazon-gamelift-servers-game-server-wrapper")
	if err := copyFile(wrapperBin, wrapperDst); err != nil {
		result.Error = fmt.Errorf("copying wrapper binary: %w", err)
		return result, result.Error
	}
	defer os.Remove(wrapperDst)

	// 3. Write wrapper config.yaml into server build directory
	wrapperConfigPath := filepath.Join(b.opts.ServerBuildDir, "config.yaml")
	if err := os.WriteFile(wrapperConfigPath, []byte(b.GenerateWrapperConfig()), 0644); err != nil {
		result.Error = fmt.Errorf("writing wrapper config: %w", err)
		return result, result.Error
	}
	defer os.Remove(wrapperConfigPath)

	// 4. Generate Dockerfile in the server build directory
	dockerfilePath := filepath.Join(b.opts.ServerBuildDir, "Dockerfile")
	dockerfile := b.GenerateDockerfile()
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
		result.Error = fmt.Errorf("writing Dockerfile: %w", err)
		return result, result.Error
	}
	defer os.Remove(dockerfilePath)

	// 5. Generate .dockerignore to exclude debug symbols (~1.7 GB savings)
	dockerignorePath := filepath.Join(b.opts.ServerBuildDir, ".dockerignore")
	if err := os.WriteFile(dockerignorePath, []byte(b.GenerateDockerignore()), 0644); err != nil {
		result.Error = fmt.Errorf("writing .dockerignore: %w", err)
		return result, result.Error
	}
	defer os.Remove(dockerignorePath)

	// 6. Docker build
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
// Creates the ECR repository if it does not already exist.
func (b *Builder) Push(ctx context.Context, opts PushOptions) error {
	if opts.AWSAccountID == "" {
		return fmt.Errorf("AWS account ID not configured (set aws.accountId in ludus.yaml)")
	}

	ecrURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s",
		opts.AWSAccountID, opts.AWSRegion, opts.ECRRepository)

	// Ensure ECR repository exists (create if missing)
	if err := b.Runner.Run(ctx, "aws", "ecr", "describe-repositories",
		"--repository-names", opts.ECRRepository,
		"--region", opts.AWSRegion); err != nil {
		fmt.Printf("    ECR repository %q not found, creating...\n", opts.ECRRepository)
		if err := b.Runner.Run(ctx, "aws", "ecr", "create-repository",
			"--repository-name", opts.ECRRepository,
			"--region", opts.AWSRegion,
			"--image-scanning-configuration", "scanOnPush=true"); err != nil {
			return fmt.Errorf("creating ECR repository: %w", err)
		}
	}

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
