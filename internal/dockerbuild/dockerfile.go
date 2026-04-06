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

// installDepsSnippet returns the RUN block that installs UE5 build prerequisites.
// Shared between builder and runtime stages.
func installDepsSnippet() string {
	return `RUN set -e; \
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
    fi`
}

// GenerateEngineDockerfile returns a multi-stage Dockerfile that builds UE5
// from source. The build context should be the engine source directory.
//
// Stage 1 (builder): installs deps, copies source, compiles everything, then
// strips Intermediate/ directories to shed ~50-100 GB of object files.
//
// Stage 2 (runtime): installs the same deps (game builds need compilers),
// then copies the cleaned engine tree from the builder in multiple layers
// so that Docker exports smaller, more reliable image layers.
func GenerateEngineDockerfile(opts DockerfileOptions) string {
	maxJobs := opts.MaxJobs
	if maxJobs <= 0 {
		maxJobs = 4
	}

	baseImage := opts.BaseImage
	if baseImage == "" {
		baseImage = "ubuntu:22.04"
	}

	deps := installDepsSnippet()

	return fmt.Sprintf(`# ----- Stage 1: builder (compile UE5 from source) -----
FROM %[1]s AS builder

%[2]s

ARG MAX_JOBS=%[3]d

COPY . /engine

WORKDIR /engine

RUN bash Setup.sh \
    && bash GenerateProjectFiles.sh \
    && make -j${MAX_JOBS} ShaderCompileWorker \
    && make -j${MAX_JOBS} UnrealEditor

# Strip build intermediates (object files) to reduce the copy to the runtime stage.
RUN find /engine -type d -name Intermediate -exec rm -rf {} + 2>/dev/null; true

# ----- Stage 2: runtime (slim image for game builds) -----
FROM %[1]s

# Game builds (BuildCookRun) invoke the compiler, so the same deps are needed.
%[2]s

WORKDIR /engine

# Copy the compiled engine in separate layers so Docker can export each one
# independently, avoiding containerd lease timeouts on very large single layers.
COPY --from=builder /engine/Engine/Binaries       /engine/Engine/Binaries
COPY --from=builder /engine/Engine/Build           /engine/Engine/Build
COPY --from=builder /engine/Engine/Config          /engine/Engine/Config
COPY --from=builder /engine/Engine/Content         /engine/Engine/Content
COPY --from=builder /engine/Engine/Plugins         /engine/Engine/Plugins
COPY --from=builder /engine/Engine/Programs        /engine/Engine/Programs
COPY --from=builder /engine/Engine/Shaders         /engine/Engine/Shaders
COPY --from=builder /engine/Engine/Source          /engine/Engine/Source
COPY --from=builder /engine/Engine/Extras          /engine/Engine/Extras
COPY --from=builder /engine/Samples                /engine/Samples
COPY --from=builder /engine/Templates              /engine/Templates
COPY --from=builder /engine/Setup.sh               /engine/Setup.sh
COPY --from=builder /engine/GenerateProjectFiles.sh /engine/GenerateProjectFiles.sh
COPY --from=builder /engine/Makefile               /engine/Makefile
`, baseImage, deps, maxJobs)
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
