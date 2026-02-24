package dockerbuild

import "fmt"

// DockerfileOptions configures engine Dockerfile generation.
type DockerfileOptions struct {
	// MaxJobs is the default compile parallelism baked into the image.
	MaxJobs int
}

// GenerateEngineDockerfile returns a Dockerfile that builds UE5 from source.
// The build context should be the engine source directory.
func GenerateEngineDockerfile(opts DockerfileOptions) string {
	maxJobs := opts.MaxJobs
	if maxJobs <= 0 {
		maxJobs = 4
	}

	return fmt.Sprintf(`FROM ubuntu:22.04

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install UE5 build prerequisites
RUN apt-get update && apt-get install -y \
    build-essential \
    git \
    cmake \
    python3 \
    curl \
    xdg-user-dirs \
    shared-mime-info \
    libfontconfig1 \
    libfreetype6 \
    libc6-dev \
    && rm -rf /var/lib/apt/lists/*

# Parallelism is configurable at build time via --build-arg
ARG MAX_JOBS=%d

# Copy engine source into the image
COPY . /engine

WORKDIR /engine

# Run Setup.sh, generate project files, and compile
RUN bash Setup.sh \
    && bash GenerateProjectFiles.sh \
    && make -j${MAX_JOBS} ShaderCompileWorker \
    && make -j${MAX_JOBS} UnrealEditor
`, maxJobs)
}

// GenerateEngineDockerignore returns a .dockerignore to reduce build context size.
func GenerateEngineDockerignore() string {
	return `# Version control
.git
.github
.gitignore
.gitattributes

# Documentation
*.md
LICENSE

# IDE files
.vscode
.idea
*.sln
*.xcodeproj
*.xcworkspace
`
}
