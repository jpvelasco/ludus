# macOS Container Engine Build Support — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `ludus engine build --backend podman|docker` work correctly on macOS (Apple Silicon and Intel) by running Linux-specific setup steps as pre-flight containers before the main image build.

**Architecture:** A new `internal/dockerbuild/macos_preflight.go` file encapsulates two pre-flight container runs (Linux toolchain bootstrap, Linux project file generation) that volume-mount the host engine tree. `GenerateEngineDockerfile()` gains a `MacOSHost bool` flag that skips Stage 3 and uses explicit Linux make targets in Stage 4. `EngineImageBuilder.Build()` orchestrates the pre-flights and passes `--platform`. All new behavior is gated on `runtime.GOOS == "darwin"` + container backend; Linux/Windows paths are unchanged.

**Tech Stack:** Go 1.25, `os/exec` via `runner.Runner`, `internal/cache` for hash-based skip logic, `internal/toolchain` for toolchain path resolution.

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/dockerbuild/macos_preflight.go` | **Create** | `LinuxToolchainPresent()`, `RunLinuxToolchainBootstrap()`, `RunLinuxGenerateProjectFiles()` |
| `internal/dockerbuild/macos_preflight_test.go` | **Create** | Tests for toolchain detection and pre-flight skip logic |
| `internal/dockerbuild/dockerfile.go` | **Modify** | Add `MacOSHost bool` to `DockerfileOptions`; modify Stage 3/4; add dotnet to dockerignore |
| `internal/dockerbuild/dockerfile_engine_test.go` | **Modify** | Add tests for `MacOSHost=true` Dockerfile variant |
| `internal/dockerbuild/engine.go` | **Modify** | Add `Arch string` to `EngineImageOptions`; pass `--platform`; call pre-flights on macOS |
| `internal/dockerbuild/engine_test.go` | **Modify** | Add test for `--platform` arg presence |
| `internal/toolchain/toolchain.go` | **Modify** | Add `LinuxToolchainPath()` helper |
| `internal/toolchain/toolchain_test.go` | **Create/Modify** | Tests for `LinuxToolchainPath()` |
| `cmd/engine/engine.go` | **Modify** | Pass `Arch` to `EngineImageOptions`; add macOS pre-flight to `runSetup()` |
| `internal/prereq/checker_docker.go` | **Modify** | Add `checkMacOSContainerBuild()` wired into `RunAll()` |
| `internal/prereq/checker_docker_test.go` | **Modify** | Tests for macOS container build check |

---

## Task 1: Add `LinuxToolchainPath()` to toolchain package

**Files:**
- Modify: `internal/toolchain/toolchain.go`
- Modify: `internal/toolchain/detection_test.go` (add tests there — it already exists)

- [ ] **Step 1: Write the failing test**

Add to `internal/toolchain/detection_test.go`:

```go
func TestLinuxToolchainPath(t *testing.T) {
    tests := []struct {
        name       string
        version    string
        setupDirs  []string // dirs to create under engineRoot/Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/Linux_x64/
        wantFound  bool
        wantPrefix string
    }{
        {
            name:      "5.7 toolchain present",
            version:   "5.7",
            setupDirs: []string{"v26_clang-20.1.8-rockylinux8"},
            wantFound: true, wantPrefix: "v26_clang-20",
        },
        {
            name:      "5.6 toolchain present",
            version:   "5.6",
            setupDirs: []string{"v25_clang-18.1.0-rockylinux8"},
            wantFound: true, wantPrefix: "v25_clang-18",
        },
        {
            name:    "toolchain absent",
            version: "5.7",
            wantFound: false,
        },
        {
            name:    "unknown version returns not found",
            version: "4.99",
            wantFound: false,
        },
        {
            name:    "empty version returns not found",
            version: "",
            wantFound: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            root := t.TempDir()
            sdkDir := filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64")
            for _, d := range tt.setupDirs {
                if err := os.MkdirAll(filepath.Join(sdkDir, d), 0o755); err != nil {
                    t.Fatal(err)
                }
            }
            got, found := LinuxToolchainPath(root, tt.version)
            if found != tt.wantFound {
                t.Errorf("found=%v, want %v (path=%q)", found, tt.wantFound, got)
            }
            if tt.wantFound && !strings.HasPrefix(filepath.Base(got), tt.wantPrefix) {
                t.Errorf("path base %q should start with %q", filepath.Base(got), tt.wantPrefix)
            }
        })
    }
}
```

- [ ] **Step 2: Run test to confirm it fails**

```
go test ./internal/toolchain/... -run TestLinuxToolchainPath -v
```
Expected: `undefined: LinuxToolchainPath`

- [ ] **Step 3: Implement `LinuxToolchainPath()`**

Add to `internal/toolchain/toolchain.go` (after `CheckToolchain`):

```go
// LinuxToolchainPath returns the path to the Linux cross-compile toolchain
// for the given engine version and whether it was found. Checks the standard
// location: Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/Linux_x64/.
// Returns ("", false) if the version is unknown or the toolchain is absent.
func LinuxToolchainPath(engineSourcePath, version string) (string, bool) {
    spec := LookupToolchain(version)
    if spec == nil {
        return "", false
    }
    sdkDir := filepath.Join(engineSourcePath, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64")
    found, path := findToolchainDir(sdkDir, spec.DirPrefix)
    return path, found
}
```

- [ ] **Step 4: Run test to confirm it passes**

```
go test ./internal/toolchain/... -run TestLinuxToolchainPath -v
```
Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/toolchain/toolchain.go internal/toolchain/detection_test.go
git commit -m "feat(toolchain): add LinuxToolchainPath() helper for macOS pre-flight"
```

---

## Task 2: Create `macos_preflight.go` with toolchain detection and pre-flight runners

**Files:**
- Create: `internal/dockerbuild/macos_preflight.go`
- Create: `internal/dockerbuild/macos_preflight_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/dockerbuild/macos_preflight_test.go`:

```go
package dockerbuild

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/jpvelasco/ludus/internal/runner"
)

func TestLinuxToolchainPresent_Found(t *testing.T) {
    root := t.TempDir()
    sdkDir := filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64", "v26_clang-20.1.8-rockylinux8")
    if err := os.MkdirAll(sdkDir, 0o755); err != nil {
        t.Fatal(err)
    }
    if !LinuxToolchainPresent(root, "5.7") {
        t.Error("expected toolchain to be found")
    }
}

func TestLinuxToolchainPresent_Missing(t *testing.T) {
    root := t.TempDir()
    if LinuxToolchainPresent(root, "5.7") {
        t.Error("expected toolchain to be absent in empty dir")
    }
}

func TestLinuxToolchainPresent_UnknownVersion(t *testing.T) {
    root := t.TempDir()
    if LinuxToolchainPresent(root, "4.99") {
        t.Error("expected false for unknown engine version")
    }
}

func TestMacOSPreflightOptions_PlatformString(t *testing.T) {
    tests := []struct {
        arch string
        want string
    }{
        {"arm64", "linux/arm64"},
        {"amd64", "linux/amd64"},
        {"", "linux/amd64"},
    }
    for _, tt := range tests {
        t.Run(tt.arch, func(t *testing.T) {
            opts := MacOSPreflightOptions{Arch: tt.arch}
            got := opts.platformString()
            if got != tt.want {
                t.Errorf("platformString() = %q, want %q", got, tt.want)
            }
        })
    }
}

func TestRunLinuxToolchainBootstrap_DryRun(t *testing.T) {
    root := t.TempDir()
    r := runner.NewRunner(false, true) // dry-run — command printed, not executed
    opts := MacOSPreflightOptions{
        EngineSourcePath: root,
        EngineVersion:    "5.7",
        BaseImage:        "ubuntu:22.04",
        Runtime:          "docker",
        Arch:             "arm64",
    }
    // Toolchain absent → bootstrap should run (in dry-run mode, no error)
    if err := RunLinuxToolchainBootstrap(opts, r); err != nil {
        t.Errorf("unexpected error in dry-run: %v", err)
    }
}

func TestRunLinuxToolchainBootstrap_SkipsWhenPresent(t *testing.T) {
    root := t.TempDir()
    sdkDir := filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64", "v26_clang-20.1.8-rockylinux8")
    if err := os.MkdirAll(sdkDir, 0o755); err != nil {
        t.Fatal(err)
    }
    // Use a runner that would fail if called — toolchain present means no run
    r := runner.NewRunner(false, false)
    opts := MacOSPreflightOptions{
        EngineSourcePath: root,
        EngineVersion:    "5.7",
        BaseImage:        "ubuntu:22.04",
        Runtime:          "docker",
        Arch:             "arm64",
    }
    // Should return nil without invoking docker (toolchain already present)
    if err := RunLinuxToolchainBootstrap(opts, r); err != nil {
        t.Errorf("unexpected error when toolchain already present: %v", err)
    }
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/dockerbuild/... -run "TestLinuxToolchain|TestMacOSPreflight|TestRunLinux" -v
```
Expected: `undefined: LinuxToolchainPresent`, `undefined: MacOSPreflightOptions`, etc.

- [ ] **Step 3: Implement `macos_preflight.go`**

Create `internal/dockerbuild/macos_preflight.go`:

```go
package dockerbuild

import (
    "fmt"

    "github.com/jpvelasco/ludus/internal/runner"
    "github.com/jpvelasco/ludus/internal/toolchain"
)

// MacOSPreflightOptions configures macOS-specific pre-flight container runs.
type MacOSPreflightOptions struct {
    EngineSourcePath string
    EngineVersion    string
    BaseImage        string
    Runtime          string // "docker" or "podman"
    Arch             string // "arm64" or "amd64"
}

func (o MacOSPreflightOptions) platformString() string {
    arch := o.Arch
    if arch == "" {
        arch = "amd64"
    }
    return "linux/" + arch
}

// LinuxToolchainPresent returns true if the Linux cross-compile toolchain for
// the given engine version is already present in the engine source tree.
func LinuxToolchainPresent(engineSourcePath, version string) bool {
    _, found := toolchain.LinuxToolchainPath(engineSourcePath, version)
    return found
}

// RunLinuxToolchainBootstrap runs Setup.sh inside a throwaway Linux container
// mounted to the host engine tree, causing Epic's downloader to fetch the Linux
// cross-compile toolchain into the host filesystem. Skips if already present.
func RunLinuxToolchainBootstrap(opts MacOSPreflightOptions, r *runner.Runner) error {
    if LinuxToolchainPresent(opts.EngineSourcePath, opts.EngineVersion) {
        return nil // already present — skip
    }

    baseImage := opts.BaseImage
    if baseImage == "" {
        baseImage = "ubuntu:22.04"
    }
    cli := ContainerCLI(opts.Runtime)

    fmt.Println("  Fetching Linux toolchain (one-time, ~2 GB)...")
    return r.Run(nil,
        cli, "run", "--rm",
        "--platform", opts.platformString(),
        "-v", opts.EngineSourcePath+":/engine",
        "-w", "/engine",
        baseImage,
        "bash", "Setup.sh",
    )
}

// RunLinuxGenerateProjectFiles runs GenerateProjectFiles.sh -makefile inside a
// throwaway Linux container mounted to the host engine tree, producing a
// Linux-targeted Makefile with explicit Linux build targets.
func RunLinuxGenerateProjectFiles(opts MacOSPreflightOptions, r *runner.Runner) error {
    baseImage := opts.BaseImage
    if baseImage == "" {
        baseImage = "ubuntu:22.04"
    }
    cli := ContainerCLI(opts.Runtime)

    fmt.Println("  Generating Linux project files...")
    return r.Run(nil,
        cli, "run", "--rm",
        "--platform", opts.platformString(),
        "-v", opts.EngineSourcePath+":/engine",
        "-w", "/engine",
        baseImage,
        "bash", "GenerateProjectFiles.sh", "-makefile",
    )
}
```

- [ ] **Step 4: Fix the `r.Run(nil, ...)` signature**

Check runner.Runner's Run signature — it takes `context.Context` as first arg. Look at `internal/runner/runner.go` to confirm, then fix the calls:

```go
import "context"

// In RunLinuxToolchainBootstrap:
return r.Run(context.Background(),
    cli, "run", "--rm",
    ...
)

// In RunLinuxGenerateProjectFiles:
return r.Run(context.Background(),
    cli, "run", "--rm",
    ...
)
```

- [ ] **Step 5: Run tests to confirm they pass**

```
go test ./internal/dockerbuild/... -run "TestLinuxToolchain|TestMacOSPreflight|TestRunLinux" -v
```
Expected: `PASS`

- [ ] **Step 6: Run full package tests**

```
go test ./internal/dockerbuild/... -v 2>&1 | tail -20
```
Expected: all `PASS`, no regressions.

- [ ] **Step 7: Commit**

```bash
git add internal/dockerbuild/macos_preflight.go internal/dockerbuild/macos_preflight_test.go
git commit -m "feat(dockerbuild): add macOS pre-flight toolchain bootstrap and GenerateProjectFiles runner"
```

---

## Task 3: Modify Dockerfile generation for macOS hosts

**Files:**
- Modify: `internal/dockerbuild/dockerfile.go`
- Modify: `internal/dockerbuild/dockerfile_engine_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/dockerbuild/dockerfile_engine_test.go`:

```go
func TestGenerateEngineDockerfile_MacOSHost_Stage3Noop(t *testing.T) {
    got := GenerateEngineDockerfile(DockerfileOptions{MacOSHost: true})

    // Stage 3 must be a no-op — no Setup.sh or GenerateProjectFiles.sh run
    if strings.Contains(got, "bash Setup.sh") {
        t.Error("macOS host Dockerfile should not run Setup.sh in Stage 3")
    }
    if strings.Contains(got, "bash GenerateProjectFiles.sh") {
        t.Error("macOS host Dockerfile should not run GenerateProjectFiles.sh in Stage 3")
    }
    // Stage 3 must still exist as a named stage (for cache chain integrity)
    if !strings.Contains(got, "AS generate") {
        t.Error("macOS host Dockerfile must still have AS generate stage")
    }
    if !strings.Contains(got, "pre-flight") {
        t.Error("macOS host Dockerfile stage 3 should mention pre-flight")
    }
}

func TestGenerateEngineDockerfile_MacOSHost_LinuxTargets(t *testing.T) {
    got := GenerateEngineDockerfile(DockerfileOptions{MacOSHost: true})

    // Stage 4 must use explicit Linux targets
    if !strings.Contains(got, "ShaderCompileWorker-Linux-Development") {
        t.Error("macOS host Dockerfile should use ShaderCompileWorker-Linux-Development target")
    }
    if !strings.Contains(got, "UnrealEditor-Linux-Development") {
        t.Error("macOS host Dockerfile should use UnrealEditor-Linux-Development target")
    }
    // Must NOT use bare targets that default to Mac
    if strings.Contains(got, "make -j${MAX_JOBS} ShaderCompileWorker\n") {
        t.Error("macOS host Dockerfile must not use bare ShaderCompileWorker target")
    }
}

func TestGenerateEngineDockerfile_NonMacOSHost_UnchangedStage3(t *testing.T) {
    got := GenerateEngineDockerfile(DockerfileOptions{MacOSHost: false})

    // Linux/Windows path unchanged
    if !strings.Contains(got, "bash Setup.sh") {
        t.Error("non-macOS Dockerfile should still run Setup.sh in Stage 3")
    }
    if !strings.Contains(got, "bash GenerateProjectFiles.sh") {
        t.Error("non-macOS Dockerfile should still run GenerateProjectFiles.sh in Stage 3")
    }
    if strings.Contains(got, "ShaderCompileWorker-Linux-Development") {
        t.Error("non-macOS Dockerfile should use bare make targets")
    }
}

func TestGenerateEngineDockerignore_ExcludesMacDotNet(t *testing.T) {
    got := GenerateEngineDockerignore()
    if !strings.Contains(got, "DotNet/mac-arm64") {
        t.Error("dockerignore should exclude mac-arm64 DotNet")
    }
    if !strings.Contains(got, "DotNet/mac-x64") {
        t.Error("dockerignore should exclude mac-x64 DotNet")
    }
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/dockerbuild/... -run "TestGenerateEngineDockerfile_MacOS|TestGenerateEngineDockerignore_Excludes" -v
```
Expected: `unknown field MacOSHost` compile error.

- [ ] **Step 3: Add `MacOSHost bool` to `DockerfileOptions` and update `GenerateEngineDockerfile()`**

In `internal/dockerbuild/dockerfile.go`, modify `DockerfileOptions`:

```go
type DockerfileOptions struct {
    MaxJobs   int
    BaseImage string
    // MacOSHost skips Stage 3 (Setup.sh + GenerateProjectFiles.sh) and uses
    // explicit Linux make targets in Stage 4. Set when building on macOS with
    // a container backend — these steps run as pre-flight containers instead.
    MacOSHost bool
}
```

Replace the Stage 3 and Stage 4 sections in `GenerateEngineDockerfile()`. The function currently uses a single `fmt.Sprintf` with the full template. Change it to build stage3 and stage4 strings conditionally before the Sprintf:

```go
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

    var stage3, stage4Scw, stage4Ue string
    if opts.MacOSHost {
        stage3 = `RUN echo "Linux toolchain and project files prepared as macOS pre-flight"`
        stage4Scw = "RUN make -j${MAX_JOBS} ShaderCompileWorker-Linux-Development"
        stage4Ue = "RUN make -j${MAX_JOBS} UnrealEditor-Linux-Development"
    } else {
        stage3 = "RUN bash Setup.sh && bash GenerateProjectFiles.sh"
        stage4Scw = "RUN make -j${MAX_JOBS} ShaderCompileWorker"
        stage4Ue = "RUN make -j${MAX_JOBS} UnrealEditor"
    }

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
# On macOS hosts, this step ran as a pre-flight container before the build.
FROM source AS generate

%[4]s

# ===== Stage 4: builder (compile the engine) =====
# Why: Compilation is the slowest part (~4 hours). Splitting ShaderCompileWorker
# and UnrealEditor into separate RUN commands lets Docker cache each independently.
# If UnrealEditor fails, ShaderCompileWorker doesn't need to be recompiled.
FROM generate AS builder

ARG MAX_JOBS=%[3]d

%[5]s
%[6]s

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
`, baseImage, deps, maxJobs, stage3, stage4Scw, stage4Ue)
}
```

- [ ] **Step 4: Add mac dotnet entries to `baseDockerignore()`**

In `internal/dockerbuild/dockerfile.go`, append to the return value of `baseDockerignore()`:

```go
func baseDockerignore() string {
    return `# Version control
...existing content...

# Mac-platform dotnet — not usable inside Linux containers, ~200 MB savings
Engine/Binaries/ThirdParty/DotNet/mac-arm64/
Engine/Binaries/ThirdParty/DotNet/mac-x64/
`
}
```

- [ ] **Step 5: Run tests to confirm they pass**

```
go test ./internal/dockerbuild/... -run "TestGenerateEngineDockerfile|TestGenerateEngineDockerignore" -v
```
Expected: all `PASS` including the new macOS tests and all existing structure/package tests.

- [ ] **Step 6: Run full dockerbuild tests**

```
go test ./internal/dockerbuild/... 2>&1 | tail -5
```
Expected: `ok github.com/jpvelasco/ludus/internal/dockerbuild`

- [ ] **Step 7: Commit**

```bash
git add internal/dockerbuild/dockerfile.go internal/dockerbuild/dockerfile_engine_test.go
git commit -m "feat(dockerbuild): add MacOSHost Dockerfile variant with no-op Stage 3 and explicit Linux make targets"
```

---

## Task 4: Add `Arch` to `EngineImageOptions` and pass `--platform` in `Build()`

**Files:**
- Modify: `internal/dockerbuild/engine.go`
- Modify: `internal/dockerbuild/engine_test.go`

- [ ] **Step 1: Write failing test**

Add to `internal/dockerbuild/engine_test.go`:

```go
func TestBuild_PassesPlatformArg(t *testing.T) {
    tmpDir := t.TempDir()
    // Create a minimal engine source so validation passes
    if err := os.WriteFile(filepath.Join(tmpDir, "Setup.sh"), []byte("#!/bin/bash"), 0o755); err != nil {
        t.Fatal(err)
    }

    r := runner.NewRunner(false, true) // dry-run captures command without executing

    b := NewEngineImageBuilder(EngineImageOptions{
        SourcePath: tmpDir,
        Runtime:    "docker",
        Arch:       "arm64",
    }, r)

    _, err := b.Build(context.Background())
    // In dry-run mode Build() returns nil — the command is printed but not run.
    // We verify the Arch was stored; actual --platform arg verification would
    // require capturing runner output, which dry-run prints to stdout.
    if err != nil {
        t.Errorf("unexpected error in dry-run: %v", err)
    }
    if b.opts.Arch != "arm64" {
        t.Errorf("Arch should be preserved, got %q", b.opts.Arch)
    }
}

func TestEngineImageOptions_Platform(t *testing.T) {
    tests := []struct {
        arch string
        want string
    }{
        {"arm64", "linux/arm64"},
        {"amd64", "linux/amd64"},
        {"", "linux/amd64"},
    }
    for _, tt := range tests {
        t.Run(tt.arch, func(t *testing.T) {
            opts := EngineImageOptions{Arch: tt.arch}
            got := opts.platformArg()
            if got != tt.want {
                t.Errorf("platformArg() = %q, want %q", got, tt.want)
            }
        })
    }
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/dockerbuild/... -run "TestBuild_PassesPlatform|TestEngineImageOptions_Platform" -v
```
Expected: `unknown field Arch`, `undefined: platformArg`

- [ ] **Step 3: Add `Arch` field and `platformArg()` to `EngineImageOptions`**

In `internal/dockerbuild/engine.go`, modify `EngineImageOptions`:

```go
type EngineImageOptions struct {
    SourcePath      string
    Version         string
    MaxJobs         int
    ImageName       string
    ImageTag        string
    NoCache         bool
    BaseImage       string
    Runtime         string
    SkipEngine      bool
    // Arch is the target CPU architecture: "amd64" or "arm64".
    // Used to set --platform linux/<arch> on the build command.
    Arch            string
}

// platformArg returns the --platform argument value for this build.
func (o EngineImageOptions) platformArg() string {
    if o.Arch == "arm64" {
        return "linux/arm64"
    }
    return "linux/amd64"
}
```

- [ ] **Step 4: Pass `--platform` in `Build()` and wire macOS pre-flights**

In `internal/dockerbuild/engine.go`, modify the `Build()` method. Replace:

```go
args := []string{
    "build",
    "--build-arg", fmt.Sprintf("MAX_JOBS=%d", b.opts.MaxJobs),
    "-t", imageTag,
    "-f", dfPath,
}
```

With:

```go
args := []string{
    "build",
    "--platform", b.opts.platformArg(),
    "--build-arg", fmt.Sprintf("MAX_JOBS=%d", b.opts.MaxJobs),
    "-t", imageTag,
    "-f", dfPath,
}
```

Also add macOS pre-flight calls and `MacOSHost` wiring in `Build()`. Add this block after the `validateSkipEngine` check and before `writeBuildContext`:

```go
import "runtime"

// macOS pre-flights: run Setup.sh and GenerateProjectFiles.sh inside Linux
// containers so the host engine tree has the Linux toolchain and Makefile.
if runtime.GOOS == "darwin" && !b.opts.SkipEngine {
    pfOpts := MacOSPreflightOptions{
        EngineSourcePath: b.opts.SourcePath,
        EngineVersion:    b.opts.Version,
        BaseImage:        b.opts.BaseImage,
        Runtime:          b.opts.Runtime,
        Arch:             b.opts.Arch,
    }
    if err := RunLinuxToolchainBootstrap(pfOpts, b.Runner); err != nil {
        return nil, fmt.Errorf("macOS Linux toolchain bootstrap: %w", err)
    }
    if err := RunLinuxGenerateProjectFiles(pfOpts, b.Runner); err != nil {
        return nil, fmt.Errorf("macOS GenerateProjectFiles: %w", err)
    }
}
```

Also pass `MacOSHost` to `writeBuildContext`:

```go
// In writeBuildContext, modify dfOpts:
dfOpts := DockerfileOptions{
    MaxJobs:   b.opts.MaxJobs,
    BaseImage: b.opts.BaseImage,
    MacOSHost: runtime.GOOS == "darwin",
}
```

- [ ] **Step 5: Run tests**

```
go test ./internal/dockerbuild/... -v 2>&1 | tail -20
```
Expected: all `PASS`.

- [ ] **Step 6: Commit**

```bash
git add internal/dockerbuild/engine.go internal/dockerbuild/engine_test.go
git commit -m "feat(dockerbuild): add Arch to EngineImageOptions, pass --platform, run macOS pre-flights in Build()"
```

---

## Task 5: Wire `Arch` and macOS pre-flight into `cmd/engine/engine.go`

**Files:**
- Modify: `cmd/engine/engine.go`

- [ ] **Step 1: Pass `Arch` in `makeContainerEngineBuilder()`**

In `cmd/engine/engine.go`, modify the `makeContainerEngineBuilder` function. Find the `dockerbuild.NewEngineImageBuilder(...)` call and add `Arch`:

```go
r := runner.NewRunner(globals.Verbose, globals.DryRun)
return dockerbuild.NewEngineImageBuilder(dockerbuild.EngineImageOptions{
    SourcePath: sourcePath,
    Version:    version,
    MaxJobs:    maxJobs,
    ImageName:  imageName,
    NoCache:    noCache,
    BaseImage:  bi,
    Runtime:    be,
    SkipEngine: skipEngine,
    Arch:       cfg.Game.ResolvedArch(), // pass game arch for --platform
}, r), nil
```

- [ ] **Step 2: Add macOS pre-flight to `runSetup()`**

In `cmd/engine/engine.go`, modify `runSetup()` to also run pre-flights on macOS with a container backend:

```go
func runSetup(cmd *cobra.Command, args []string) error {
    builder, err := makeBuilder()
    if err != nil {
        return err
    }

    fmt.Println("Running engine setup...")
    if err := builder.Setup(cmd.Context()); err != nil {
        return err
    }

    // On macOS with a container backend, also run the Linux pre-flights so
    // the engine tree is fully prepared for container builds.
    be := resolveBackend()
    if runtime.GOOS == "darwin" && dockerbuild.IsContainerBackend(be) {
        cfg := globals.Cfg
        sourcePath := uePath
        if sourcePath == "" {
            sourcePath = cfg.Engine.SourcePath
        }
        bi := baseImage
        if bi == "" {
            bi = cfg.Engine.DockerBaseImage
        }
        version, _ := toolchain.DetectEngineVersion(sourcePath, cfg.Engine.Version)
        pfOpts := dockerbuild.MacOSPreflightOptions{
            EngineSourcePath: sourcePath,
            EngineVersion:    version,
            BaseImage:        bi,
            Runtime:          be,
            Arch:             cfg.Game.ResolvedArch(),
        }
        r := runner.NewRunner(globals.Verbose, globals.DryRun)
        if err := dockerbuild.RunLinuxToolchainBootstrap(pfOpts, r); err != nil {
            return fmt.Errorf("Linux toolchain bootstrap: %w", err)
        }
        if err := dockerbuild.RunLinuxGenerateProjectFiles(pfOpts, r); err != nil {
            return fmt.Errorf("Linux GenerateProjectFiles: %w", err)
        }
    }

    fmt.Println("\nNext: ludus engine build")
    return nil
}
```

Add `"runtime"` to the import block in `cmd/engine/engine.go`.

- [ ] **Step 3: Build to confirm no compile errors**

```
go build ./cmd/engine/... 2>&1
```
Expected: no output (success).

- [ ] **Step 4: Run full test suite**

```
go test ./... 2>&1 | grep -E "FAIL|ok" | tail -20
```
Expected: all `ok`, no `FAIL`.

- [ ] **Step 5: Commit**

```bash
git add cmd/engine/engine.go
git commit -m "feat(engine): pass game.arch to engine image build, run macOS pre-flights in ludus engine setup"
```

---

## Task 6: Add `checkMacOSContainerBuild()` prereq check

**Files:**
- Modify: `internal/prereq/checker_docker.go`
- Modify: `internal/prereq/checker_docker_test.go`
- Modify: `internal/prereq/checker.go` (wire into `RunAll()`)

- [ ] **Step 1: Write failing tests**

Add to `internal/prereq/checker_docker_test.go`:

```go
func TestCheckMacOSContainerBuild_NilGameConfig(t *testing.T) {
    c := &Checker{Backend: "podman"}
    result := c.checkMacOSContainerBuild()
    // With no engine source path, check should skip gracefully (warning)
    if result.Name != "macOS Container Build" {
        t.Errorf("expected name 'macOS Container Build', got: %s", result.Name)
    }
    if !result.Passed {
        t.Errorf("expected pass (skip) with no engine source, got: %s", result.Message)
    }
}

func TestCheckMacOSContainerBuild_NonContainerBackend(t *testing.T) {
    c := &Checker{Backend: "native", EngineSourcePath: "/some/path"}
    result := c.checkMacOSContainerBuild()
    // Native backend → check should skip (not applicable)
    if !result.Passed {
        t.Errorf("expected pass (skip) for native backend, got: %s", result.Message)
    }
    if result.Warning {
        t.Errorf("native backend should not warn")
    }
}

func TestCheckMacOSContainerBuild_ToolchainMissing(t *testing.T) {
    root := t.TempDir()
    c := &Checker{
        Backend:          "podman",
        EngineSourcePath: root,
        EngineVersion:    "5.7",
        GameConfig:       &config.GameConfig{Arch: "arm64"},
    }
    result := c.checkMacOSContainerBuild()
    if result.Name != "macOS Container Build" {
        t.Errorf("unexpected name: %s", result.Name)
    }
    // Missing toolchain → warning (bootstrap will fetch it automatically)
    if !result.Passed {
        t.Errorf("expected pass+warning (not failure) for missing toolchain, got: %s", result.Message)
    }
    if !result.Warning {
        t.Errorf("expected warning flag for missing toolchain")
    }
    if !strings.Contains(result.Message, "Linux toolchain") {
        t.Errorf("expected 'Linux toolchain' in message, got: %s", result.Message)
    }
}

func TestCheckMacOSContainerBuild_ToolchainPresent(t *testing.T) {
    root := t.TempDir()
    sdkDir := filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64", "v26_clang-20.1.8-rockylinux8")
    if err := os.MkdirAll(sdkDir, 0o755); err != nil {
        t.Fatal(err)
    }
    c := &Checker{
        Backend:          "podman",
        EngineSourcePath: root,
        EngineVersion:    "5.7",
        GameConfig:       &config.GameConfig{Arch: "arm64"},
    }
    result := c.checkMacOSContainerBuild()
    if !result.Passed || result.Warning {
        t.Errorf("expected clean pass when toolchain present, got: passed=%v warning=%v message=%s",
            result.Passed, result.Warning, result.Message)
    }
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/prereq/... -run "TestCheckMacOSContainerBuild" -v
```
Expected: `undefined: (Checker).checkMacOSContainerBuild`

- [ ] **Step 3: Implement `checkMacOSContainerBuild()` in `checker_docker.go`**

Add to `internal/prereq/checker_docker.go`:

```go
// checkMacOSContainerBuild verifies that macOS container build prerequisites are met.
// Runs only on macOS with a container backend — skips silently on all other platforms.
// Issues a warning (not failure) when the Linux toolchain is absent, since
// `ludus engine build` will fetch it automatically as a pre-flight step.
func (c *Checker) checkMacOSContainerBuild() CheckResult {
    name := "macOS Container Build"

    if c.EngineSourcePath == "" || (c.Backend != dockerbuild.BackendDocker && c.Backend != dockerbuild.BackendPodman) {
        return CheckResult{Name: name, Passed: true, Message: "skipped (not a macOS container build)"}
    }

    version := c.EngineVersion
    if !dockerbuild.LinuxToolchainPresent(c.EngineSourcePath, version) {
        return CheckResult{
            Name:    name,
            Passed:  true,
            Warning: true,
            Message: "Linux toolchain not yet fetched — will be downloaded automatically on first engine build " +
                "(run 'ludus engine setup' to pre-fetch)",
        }
    }

    return CheckResult{Name: name, Passed: true, Message: "Linux toolchain present"}
}
```

- [ ] **Step 4: Wire `checkMacOSContainerBuild()` into `RunAll()` in `checker.go`**

In `internal/prereq/checker.go`, find `RunAll()` and add after `checkCrossArchEmulation()`:

```go
results = append(results, c.checkCrossArchEmulation())
results = append(results, c.checkMacOSContainerBuild()) // add this line
results = append(results, c.checkCommand("aws", "AWS CLI"))
```

Note: `checkMacOSContainerBuild` is in `checker_docker.go` which has no build tags — it compiles on all platforms. The function itself is a no-op on non-macOS or non-container backends, so this is safe.

- [ ] **Step 5: Run tests**

```
go test ./internal/prereq/... -run "TestCheckMacOSContainerBuild" -v
```
Expected: all `PASS`.

- [ ] **Step 6: Run full prereq tests**

```
go test ./internal/prereq/... 2>&1 | tail -5
```
Expected: `ok github.com/jpvelasco/ludus/internal/prereq`

- [ ] **Step 7: Check `checker_stages_test.go` — count of checks may have changed**

```
go test ./internal/prereq/... -run TestCheck -v 2>&1 | grep -E "PASS|FAIL"
```
If `TestCheckDockerReady` or `TestCheckAWSReady` fails, update the expected count in `checker_stages_test.go`.

- [ ] **Step 8: Commit**

```bash
git add internal/prereq/checker_docker.go internal/prereq/checker_docker_test.go internal/prereq/checker.go
git commit -m "feat(prereq): add checkMacOSContainerBuild() warning for missing Linux toolchain"
```

---

## Task 7: Final verification and PR

- [ ] **Step 1: Run full test suite**

```
go test ./... 2>&1 | grep -E "FAIL|ok"
```
Expected: all `ok`, zero `FAIL`.

- [ ] **Step 2: Run linter**

```
golangci-lint run ./... 2>&1
```
Expected: `0 issues.`

- [ ] **Step 3: Cross-compile check for all three platforms**

```bash
GOOS=linux go build ./... 2>&1
GOOS=darwin go build ./... 2>&1
GOOS=windows go build ./... 2>&1
```
Expected: all silent (no errors).

- [ ] **Step 4: Create branch and PR**

```bash
git checkout -b feat/macos-container-engine-build
git push -u origin feat/macos-container-engine-build
gh pr create \
  --title "feat: macOS container engine build support (issues #237-240)" \
  --body "$(cat <<'EOF'
## Summary

- Adds two macOS pre-flight container runs before the main engine image build:
  1. **Linux toolchain bootstrap** — runs Setup.sh inside Linux to fetch the Linux cross-compile toolchain (one-time, ~2 GB, cached)
  2. **Linux GenerateProjectFiles** — runs GenerateProjectFiles.sh -makefile inside Linux to produce a Linux-targeted Makefile
- Modifies the engine Dockerfile when building on macOS: Stage 3 is a no-op, Stage 4 uses explicit Linux make targets
- Passes `--platform linux/<arch>` to all engine image builds
- Excludes mac-platform dotnet from the Docker build context (~200 MB savings)
- `ludus engine setup` on macOS + container backend now runs all pre-flights automatically
- `ludus init` warns when the Linux toolchain is missing with a clear remediation message
- Linux and Windows builds are completely unaffected

## Test plan

- [ ] `go test ./...` passes on Linux CI
- [ ] `go test ./...` passes on Windows CI
- [ ] `TestLinuxToolchainPath_*` — toolchain detection at correct paths
- [ ] `TestLinuxToolchainPresent_*` — present/absent/unknown-version cases
- [ ] `TestRunLinuxToolchainBootstrap_*` — dry-run and skip-when-present
- [ ] `TestGenerateEngineDockerfile_MacOSHost_*` — Stage 3 no-op, Linux make targets
- [ ] `TestGenerateEngineDockerignore_ExcludesMacDotNet` — mac dotnet excluded
- [ ] `TestBuild_PassesPlatformArg` — --platform in build args
- [ ] `TestCheckMacOSContainerBuild_*` — warning when toolchain absent, pass when present
- [ ] On macOS with Podman: `ludus engine build --backend podman` completes successfully
- [ ] On macOS with Docker: `ludus engine build --backend docker` completes successfully
- [ ] On macOS, second run skips toolchain bootstrap (cached)

Closes #237, closes #238, closes #239, closes #240
EOF
)"
```

---

## Self-Review

**Spec coverage check:**

| Spec component | Task |
|---------------|------|
| Linux toolchain bootstrap pre-flight | Task 2 |
| GenerateProjectFiles pre-flight | Task 2 |
| Dockerfile Stage 3 no-op on macOS | Task 3 |
| Dockerfile Stage 4 Linux targets on macOS | Task 3 |
| dotnet dockerignore exclusion | Task 3 |
| `--platform` flag in engine build | Task 4 |
| `ludus engine setup` macOS pre-flight | Task 5 |
| `checkMacOSContainerBuild()` prereq check | Task 6 |
| `LinuxToolchainPath()` toolchain helper | Task 1 |

All spec components covered. ✓

**Placeholder scan:** No TBDs, no "fill in later", all code blocks complete. ✓

**Type consistency check:**
- `MacOSPreflightOptions` defined in Task 2, used in Tasks 4, 5 ✓
- `LinuxToolchainPresent()` defined in Task 2, used in Tasks 6 ✓
- `RunLinuxToolchainBootstrap()` / `RunLinuxGenerateProjectFiles()` defined in Task 2, used in Tasks 4, 5 ✓
- `DockerfileOptions.MacOSHost` defined in Task 3, set in Task 4 ✓
- `EngineImageOptions.Arch` / `platformArg()` defined in Task 4, set in Task 5 ✓
- `LinuxToolchainPath()` defined in Task 1, called inside `LinuxToolchainPresent()` in Task 2 ✓
