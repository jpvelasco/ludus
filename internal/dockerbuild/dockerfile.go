package dockerbuild

import "fmt"

// DockerfileOptions configures engine Dockerfile generation.
type DockerfileOptions struct {
	// MaxJobs is the default compile parallelism baked into the image.
	MaxJobs int
	// BaseImage is the Docker base image (e.g. "ubuntu:22.04", "amazonlinux:2023").
	// Supports Debian/Ubuntu (apt-get) and RHEL/Amazon Linux/Fedora (dnf).
	BaseImage string
}

// GenerateEngineDockerfile returns a Dockerfile that builds UE5 from source.
// The build context should be the engine source directory.
// The Dockerfile auto-detects the package manager (apt-get vs dnf) at build time.
func GenerateEngineDockerfile(opts DockerfileOptions) string {
	maxJobs := opts.MaxJobs
	if maxJobs <= 0 {
		maxJobs = 4
	}

	baseImage := opts.BaseImage
	if baseImage == "" {
		baseImage = "ubuntu:22.04"
	}

	return fmt.Sprintf(`FROM %s

# Install UE5 build prerequisites using the available package manager.
# Supports apt-get (Debian/Ubuntu) and dnf (Amazon Linux/RHEL/Fedora).
RUN set -e; \
    if command -v apt-get >/dev/null 2>&1; then \
        export DEBIAN_FRONTEND=noninteractive; \
        apt-get update && apt-get install -y \
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
        && rm -rf /var/lib/apt/lists/*; \
    elif command -v dnf >/dev/null 2>&1; then \
        dnf install -y \
            gcc \
            gcc-c++ \
            make \
            git \
            cmake \
            python3 \
            curl \
            xdg-user-dirs \
            shared-mime-info \
            fontconfig-devel \
            freetype-devel \
            glibc-devel \
        && dnf clean all; \
    else \
        echo "ERROR: No supported package manager found (need apt-get or dnf)" >&2; \
        exit 1; \
    fi

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
`, baseImage, maxJobs)
}

// GenerateEngineDockerignore returns a .dockerignore to reduce build context size.
// UE5 source trees can be 300+ GB with host-platform build artifacts;
// this typically cuts the build context to ~50-80 GB.
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

# Host-platform build artifacts (rebuilt fresh inside the container).
# NOTE: We exclude DerivedDataCache only under specific known cache locations,
# not with **/DerivedDataCache/, because UE5 has a source module at
# Engine/Source/Developer/DerivedDataCache/ that must be included for compilation.
**/Intermediate/
**/Saved/
Engine/DerivedDataCache/

# Host-platform binaries (wrong platform for Linux container)
**/Binaries/Win64/
**/Binaries/Mac/

# Previous build outputs
**/PackagedServer/
**/PackagedClient/
`
}
