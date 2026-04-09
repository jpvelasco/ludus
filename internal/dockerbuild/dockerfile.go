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

# NOTE: Intermediate/ dirs (~50-100 GB of compiled object files) are intentionally
# kept. Stripping them would shrink the image but force a full recompile (~3000
# modules, ~5 hours) on every game build. The size tradeoff is worth it.

# ===== Stage 5: runtime (slim image for game builds via BuildCookRun) =====
# Why: Fresh base avoids carrying builder-only layers. Smaller layers mean more
# reliable Docker exports (avoids the containerd lease/unpack timeouts we hit
# with single-stage 200+ GB images).
FROM %[1]s AS runtime

# BuildCookRun invokes compilers and linkers, so build deps are still required.
%[2]s

# Non-root build user. UE 5.7+ refuses to run UnrealEditor-Cmd as root on x86_64
# (check in UnixPlatformMemory.cpp). The game build preamble switches to this user
# before running BuildCookRun.
RUN useradd -m -s /bin/bash ue

ENV UE_ROOT=/engine
ENV PATH="/engine/Engine/Binaries/Linux:${PATH}"

WORKDIR /engine

# DDC mount point for persistent derived data cache volumes.
RUN mkdir -p /ddc && chown ue:ue /ddc

# --- Compiled binaries (editor, tools, bundled runtimes) ---
COPY --chown=ue:ue --from=builder /engine/Engine/Binaries  /engine/Engine/Binaries

# --- Build system (RunUAT.sh, build scripts, UnrealBuildTool) ---
COPY --chown=ue:ue --from=builder /engine/Engine/Build     /engine/Engine/Build
COPY --chown=ue:ue --from=builder /engine/Engine/Programs  /engine/Engine/Programs

# --- Content, shaders, and configuration ---
COPY --chown=ue:ue --from=builder /engine/Engine/Config    /engine/Engine/Config
COPY --chown=ue:ue --from=builder /engine/Engine/Content   /engine/Engine/Content
COPY --chown=ue:ue --from=builder /engine/Engine/Shaders   /engine/Engine/Shaders
COPY --chown=ue:ue --from=builder /engine/Engine/Plugins   /engine/Engine/Plugins

# --- Source (AutomationTool scripts recompile during BuildCookRun) ---
COPY --chown=ue:ue --from=builder /engine/Engine/Source    /engine/Engine/Source
COPY --chown=ue:ue --from=builder /engine/Engine/Extras    /engine/Engine/Extras

# --- Sample projects (Lyra) and templates ---
COPY --chown=ue:ue --from=builder /engine/Samples          /engine/Samples
COPY --chown=ue:ue --from=builder /engine/Templates        /engine/Templates

# --- Root-level build scripts ---
COPY --chown=ue:ue --from=builder /engine/Setup.sh               /engine/Setup.sh
COPY --chown=ue:ue --from=builder /engine/GenerateProjectFiles.sh /engine/GenerateProjectFiles.sh
COPY --chown=ue:ue --from=builder /engine/Makefile               /engine/Makefile

CMD ["echo", "Ludus Engine Image Ready - use with: ludus game build --backend docker|podman"]
`, baseImage, deps, maxJobs)
}

// GeneratePrebuiltEngineDockerfile returns a 2-stage Dockerfile that packages
// pre-built Linux binaries into a container image without compiling from source.
// The build context should be the engine source directory containing compiled
// Linux binaries (Engine/Binaries/Linux/).
//
// Use this with --skip-engine to avoid the multi-hour compilation when the
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

# Non-root build user. UE 5.7+ refuses to run UnrealEditor-Cmd as root on x86_64.
RUN useradd -m -s /bin/bash ue

ENV UE_ROOT=/engine
ENV PATH="/engine/Engine/Binaries/Linux:${PATH}"

WORKDIR /engine

# DDC mount point for persistent derived data cache volumes.
RUN mkdir -p /ddc && chown ue:ue /ddc

# --- Compiled binaries (editor, tools, bundled runtimes) ---
COPY --chown=ue:ue Engine/Binaries  /engine/Engine/Binaries

# --- Build system (RunUAT.sh, build scripts, UnrealBuildTool) ---
COPY --chown=ue:ue Engine/Build     /engine/Engine/Build
COPY --chown=ue:ue Engine/Programs  /engine/Engine/Programs

# --- Content, shaders, and configuration ---
COPY --chown=ue:ue Engine/Config    /engine/Engine/Config
COPY --chown=ue:ue Engine/Content   /engine/Engine/Content
COPY --chown=ue:ue Engine/Shaders   /engine/Engine/Shaders
COPY --chown=ue:ue Engine/Plugins   /engine/Engine/Plugins

# --- Source (AutomationTool scripts recompile during BuildCookRun) ---
COPY --chown=ue:ue Engine/Source    /engine/Engine/Source
COPY --chown=ue:ue Engine/Extras    /engine/Engine/Extras

# --- Sample projects (Lyra) and templates ---
COPY --chown=ue:ue Samples          /engine/Samples
COPY --chown=ue:ue Templates        /engine/Templates

# --- Root-level build scripts ---
COPY --chown=ue:ue Setup.sh               /engine/Setup.sh
COPY --chown=ue:ue GenerateProjectFiles.sh /engine/GenerateProjectFiles.sh
COPY --chown=ue:ue Makefile               /engine/Makefile

CMD ["echo", "Ludus Engine Image Ready - use with: ludus game build --backend docker|podman"]
`, baseImage, deps)
}

// GeneratePrebuiltEngineDockerignore returns a .dockerignore for skip-engine
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
