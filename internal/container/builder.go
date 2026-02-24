package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/wrapper"
)

// BuildOptions configures the container image build.
type BuildOptions struct {
	// ServerBuildDir is the path to the packaged server.
	ServerBuildDir string
	// ImageName is the Docker image name.
	ImageName string
	// Tag is the Docker image tag.
	Tag string
	// ServerPort is the game server port.
	ServerPort int
	// NoCache disables Docker build cache.
	NoCache bool
	// ProjectName is the UE5 project name (e.g. "Lyra").
	ProjectName string
	// ServerTarget is the server binary name (e.g. "LyraServer").
	ServerTarget string
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

// resolveProjectName returns the project name, defaulting to "Lyra".
func (b *Builder) resolveProjectName() string {
	if b.opts.ProjectName != "" {
		return b.opts.ProjectName
	}
	return "Lyra"
}

// resolveServerTarget returns the server target name, defaulting to ProjectName + "Server".
func (b *Builder) resolveServerTarget() string {
	if b.opts.ServerTarget != "" {
		return b.opts.ServerTarget
	}
	return b.resolveProjectName() + "Server"
}

// ensureWrapper delegates to the shared wrapper package to clone and build
// the Amazon GameLift Game Server Wrapper binary.
func (b *Builder) ensureWrapper(ctx context.Context) (string, error) {
	return wrapper.EnsureBinary(ctx, b.Runner)
}

// GenerateWrapperConfig produces the config.yaml for the GameLift Game Server Wrapper.
// The wrapper uses this to know how to launch the game server process.
func (b *Builder) GenerateWrapperConfig() string {
	projectName := b.resolveProjectName()
	serverTarget := b.resolveServerTarget()

	return fmt.Sprintf(`log-config:
  wrapper-log-level: info

ports:
  gamePort: %d

game-server-details:
  executable-file-path: ./%s/Binaries/Linux/%s
  game-server-args:
    - arg: "%s"
      val: ""
      pos: 0
    - arg: "-port="
      val: "{{.ContainerPort}}"
      pos: 1
    - arg: "-log"
      val: ""
      pos: 2
`, b.opts.ServerPort, projectName, serverTarget, projectName)
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

// GenerateDockerfile creates a Dockerfile for the game server container.
// The packaged server from RunUAT BuildCookRun has this structure:
//
//	LinuxServer/
//	├── <ServerTarget>.sh              (launcher script)
//	├── Engine/                         (engine runtime content)
//	└── <ProjectName>/
//	    ├── Binaries/Linux/<ServerTarget>  (actual server binary)
//	    ├── Config/
//	    ├── Content/
//	    └── Plugins/
//
// Based on the GameLift Containers Starter Kit pattern.
func (b *Builder) GenerateDockerfile() string {
	projectName := b.resolveProjectName()
	serverTarget := b.resolveServerTarget()

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
    && chmod +x /opt/server/%s/Binaries/Linux/%s

# Expose game server port
EXPOSE %d/udp

# Switch to non-root user
USER ueserver
WORKDIR /opt/server

# Wrapper is PID 1 — handles GameLift SDK, launches game server as child process
ENTRYPOINT ["./amazon-gamelift-servers-game-server-wrapper"]
`, projectName, serverTarget, b.opts.ServerPort)
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

// Build creates the Docker image for the dedicated server.
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
