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
// Shared between deps and runtime stages. Both need compilers because
// BuildCookRun invokes the linker and recompiles AutomationTool scripts.
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

// GenerateEngineDockerfile returns a 5-stage Dockerfile that builds UE5 from
// source. The build context should be the engine source directory.
//
// The stages are layered for Docker cache efficiency:
//
//	Stage 1 (deps):     install build prerequisites (cached until base image changes)
//	Stage 2 (source):   copy engine source tree (invalidated on source changes)
//	Stage 3 (generate): run Setup.sh + GenerateProjectFiles.sh
//	Stage 4 (builder):  compile the engine, then strip Intermediate/ object files
//	Stage 5 (runtime):  fresh base with deps + compiled artifacts from builder
//
// The runtime stage installs the same build deps because BuildCookRun invokes
// compilers and linkers during game builds.
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

	return fmt.Sprintf(`# ===== Stage 1: deps (install build prerequisites) =====
# Why: Dependencies change rarely. Caching this stage saves significant time
# on rebuilds -- only invalidated when the base image or package list changes.
FROM %[1]s AS deps

%[2]s

# ===== Stage 2: source (copy engine source tree) =====
# Why: Source changes frequently. Everything after this stage is invalidated
# on source changes, but the deps layer above stays cached.
FROM deps AS source

WORKDIR /engine
COPY . /engine

# ===== Stage 3: generate (fetch third-party deps, create Makefiles) =====
# Why: Setup.sh downloads ~20 GB of third-party content. Separating this from
# compilation means a compile failure doesn't force re-downloading everything.
FROM source AS generate

RUN bash Setup.sh && bash GenerateProjectFiles.sh

# ===== Stage 4: builder (compile the engine) =====
# Why: Compilation is the slowest part (~4 hours). Splitting ShaderCompileWorker
# and UnrealEditor into separate RUN commands lets Docker cache each independently.
# If UnrealEditor fails, ShaderCompileWorker doesn't need to be recompiled.
FROM generate AS builder

ARG MAX_JOBS=%[3]d

RUN make -j${MAX_JOBS} ShaderCompileWorker
RUN make -j${MAX_JOBS} UnrealEditor

# Strip build intermediates (~50-100 GB of object files).
RUN find /engine -type d -name Intermediate -exec rm -rf {} + 2>/dev/null; true

# ===== Stage 5: runtime (slim image for game builds via BuildCookRun) =====
# Why: Fresh base avoids carrying builder-only layers. Smaller layers mean more
# reliable Docker exports (avoids the containerd lease/unpack timeouts we hit
# with single-stage 200+ GB images).
FROM %[1]s AS runtime

# BuildCookRun invokes compilers and linkers, so build deps are still required.
%[2]s

ENV UE_ROOT=/engine
ENV PATH="/engine/Engine/Binaries/Linux:${PATH}"

WORKDIR /engine

# DDC mount point for persistent derived data cache volumes.
RUN mkdir -p /ddc

# --- Compiled binaries (editor, tools, bundled runtimes) ---
COPY --from=builder /engine/Engine/Binaries  /engine/Engine/Binaries

# --- Build system (RunUAT.sh, build scripts, UnrealBuildTool) ---
COPY --from=builder /engine/Engine/Build     /engine/Engine/Build
COPY --from=builder /engine/Engine/Programs  /engine/Engine/Programs

# --- Content, shaders, and configuration ---
COPY --from=builder /engine/Engine/Config    /engine/Engine/Config
COPY --from=builder /engine/Engine/Content   /engine/Engine/Content
COPY --from=builder /engine/Engine/Shaders   /engine/Engine/Shaders
COPY --from=builder /engine/Engine/Plugins   /engine/Engine/Plugins

# --- Source (AutomationTool scripts recompile during BuildCookRun) ---
COPY --from=builder /engine/Engine/Source    /engine/Engine/Source
COPY --from=builder /engine/Engine/Extras    /engine/Engine/Extras

# --- Sample projects (Lyra) and templates ---
COPY --from=builder /engine/Samples          /engine/Samples
COPY --from=builder /engine/Templates        /engine/Templates

# --- Root-level build scripts ---
COPY --from=builder /engine/Setup.sh               /engine/Setup.sh
COPY --from=builder /engine/GenerateProjectFiles.sh /engine/GenerateProjectFiles.sh
COPY --from=builder /engine/Makefile               /engine/Makefile

CMD ["echo", "Ludus Engine Image Ready - use with: ludus game build --backend docker|podman"]
`, baseImage, deps, maxJobs)
}

// GeneratePrebuiltEngineDockerfile returns a 2-stage Dockerfile that packages
// pre-built Linux binaries into a container image without compiling from source.
// The build context should be the engine source directory containing compiled
// Linux binaries (Engine/Binaries/Linux/).
//
// Use this with --skip-compile to avoid the multi-hour compilation when the
// engine has already been built natively or in a previous container build.
func GeneratePrebuiltEngineDockerfile(opts DockerfileOptions) string {
	baseImage := opts.BaseImage
	if baseImage == "" {
		baseImage = "ubuntu:22.04"
	}

	deps := installDepsSnippet()

	return fmt.Sprintf(`# ===== Stage 1: deps (install build prerequisites) =====
# Why: BuildCookRun invokes compilers and linkers during game builds,
# so build deps are required even though we skip compilation here.
FROM %[1]s AS deps

%[2]s

# ===== Stage 2: runtime (package pre-built binaries) =====
# Why: Skips the compile stages entirely. Copies pre-built Linux binaries
# directly from the build context (host filesystem) into the image.
FROM deps AS runtime

ENV UE_ROOT=/engine
ENV PATH="/engine/Engine/Binaries/Linux:${PATH}"

WORKDIR /engine

# DDC mount point for persistent derived data cache volumes.
RUN mkdir -p /ddc

# --- Compiled binaries (editor, tools, bundled runtimes) ---
COPY Engine/Binaries  /engine/Engine/Binaries

# --- Build system (RunUAT.sh, build scripts, UnrealBuildTool) ---
COPY Engine/Build     /engine/Engine/Build
COPY Engine/Programs  /engine/Engine/Programs

# --- Content, shaders, and configuration ---
COPY Engine/Config    /engine/Engine/Config
COPY Engine/Content   /engine/Engine/Content
COPY Engine/Shaders   /engine/Engine/Shaders
COPY Engine/Plugins   /engine/Engine/Plugins

# --- Source (AutomationTool scripts recompile during BuildCookRun) ---
COPY Engine/Source    /engine/Engine/Source
COPY Engine/Extras    /engine/Engine/Extras

# --- Sample projects (Lyra) and templates ---
COPY Samples          /engine/Samples
COPY Templates        /engine/Templates

# --- Root-level build scripts ---
COPY Setup.sh               /engine/Setup.sh
COPY GenerateProjectFiles.sh /engine/GenerateProjectFiles.sh
COPY Makefile               /engine/Makefile

CMD ["echo", "Ludus Engine Image Ready - use with: ludus game build --backend docker|podman"]
`, baseImage, deps)
}

// GeneratePrebuiltEngineDockerignore returns a .dockerignore for skip-compile
// builds. More aggressive than the full-build ignore since we only need the
// directories that go into the runtime image.
func GeneratePrebuiltEngineDockerignore() string {
	return `# Version control
.git
.github
.gitignore
.gitattributes

# Documentation
*.md
LICENSE

# IDE and editor files
.vscode
.idea
.vs
*.sln
*.suo
*.user
*.xcodeproj
*.xcworkspace

# Build intermediates (not needed in runtime image)
**/Intermediate/
**/Saved/
Engine/DerivedDataCache/

# Host-platform binaries (wrong platform for Linux container)
**/Binaries/Win64/
**/Binaries/Mac/

# Debug symbols
**/*.pdb
**/*.dSYM

# Previous build outputs
**/PackagedServer/
**/PackagedClient/

# Directories not needed in runtime image
FeaturePacks/
`
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

# IDE and editor files
.vscode
.idea
.vs
*.sln
*.suo
*.user
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

# Windows debug symbols (wrong platform, can be 50+ GB)
**/*.pdb

# macOS debug symbols
**/*.dSYM

# Previous build outputs
**/PackagedServer/
**/PackagedClient/
`
}
