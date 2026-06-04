# macOS Container Engine Build Support

**Date:** 2026-06-03  
**Issues:** #237, #238, #239, #240  
**Status:** Approved for implementation

---

## Problem

`ludus engine build --backend podman|docker` fails completely on macOS. Four cascading failures, all rooted in one cause:

**Root cause:** `Setup.sh` run on macOS downloads the macOS toolchain, not the Linux cross-compile toolchain (`Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/`). The 5-stage Dockerfile then tries to re-run setup work inside a Linux container that cannot execute macOS binaries.

| Issue | Symptom | Root |
|-------|---------|------|
| #237 | `Setup.sh` in container invokes x86_64 ELF helpers; QEMU fails on ARM64 Linux | Stage 3 re-runs host setup inside Linux |
| #238 | `GenerateProjectFiles.sh` in container picks up mac-arm64 dotnet; can't run on Linux | Stage 3 re-runs host setup inside Linux |
| #239 | `make ShaderCompileWorker` resolves to Mac target; `RunUBT.sh` not found | Makefile generated on macOS has Mac defaults |
| #240 | `Platform Linux is not a valid platform to build` | Linux cross-compile toolchain never downloaded |

---

## Design

### Core Insight

The Dockerfile Stage 3 (`Setup.sh && GenerateProjectFiles.sh`) performs **Linux-specific work that must run on Linux**. On macOS with a container backend, this work must happen in a disposable Linux container mounted against the host engine tree — before the main image build — so the results land on disk and are available when the main build's `COPY` layer picks them up.

### Trigger Condition

All new behavior is gated on:

```
runtime.GOOS == "darwin" && (backend == "docker" || backend == "podman")
```

Linux and Windows builds are **completely unaffected**. No existing code paths change.

---

## Components

### 1. macOS Pre-flight: Linux Toolchain Bootstrap

**What:** Run `Setup.sh` inside a throwaway Linux container, volume-mounted to the host engine tree. This causes Epic's dependency downloader to fetch the Linux toolchain into the host filesystem.

**When:** Only if `Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/` does not already contain the required toolchain for the detected engine version. Cached on all subsequent runs — ludus checks existence before running.

**Command:**
```
docker|podman run --rm \
  --platform linux/arm64|linux/amd64 \
  -v <engine-source>:/engine \
  -w /engine \
  <base-image> \
  bash Setup.sh
```

**User experience:** Named progress step — `"Fetching Linux toolchain (one-time, ~2 GB)..."`. On subsequent runs this step is skipped silently (logged at `--verbose` only).

**Implementation location:** New function `runMacOSLinuxToolchainBootstrap()` in `internal/dockerbuild/macos_preflight.go`.

---

### 2. macOS Pre-flight: Generate Linux Project Files

**What:** Run `GenerateProjectFiles.sh -makefile` inside a throwaway Linux container, volume-mounted to the host engine tree. Produces a Linux-targeted `Makefile` in the engine source root.

**When:** Always runs on macOS container builds, but is hash-skipped if the engine source hash hasn't changed since the last run. Hash stored in `.ludus/cache.json` under a new key `engine-generate-linux`.

**Command:**
```
docker|podman run --rm \
  --platform linux/arm64|linux/amd64 \
  -v <engine-source>:/engine \
  -w /engine \
  <base-image> \
  bash GenerateProjectFiles.sh -makefile
```

**Why `-makefile`:** Produces a `Makefile` without VS/Xcode project files. The Linux Makefile has explicit `ShaderCompileWorker-Linux-Development` and `UnrealEditor-Linux-Development` targets — no Mac defaults.

**User experience:** Named progress step — `"Generating Linux project files..."`.

**Implementation location:** New function `runMacOSGenerateProjectFiles()` in `internal/dockerbuild/macos_preflight.go`.

---

### 3. Modified Dockerfile for macOS Hosts

**Stage 3 change:** When host is macOS + container backend, Stage 3 becomes a no-op:

```dockerfile
# ===== Stage 3: generate (pre-flight ran on host for macOS builds) =====
FROM source AS generate
RUN echo "Linux toolchain and project files prepared as macOS pre-flight"
```

**Stage 4 change:** Explicit Linux make targets replace bare `make ShaderCompileWorker`:

```dockerfile
RUN make -j${MAX_JOBS} ShaderCompileWorker-Linux-Development
RUN make -j${MAX_JOBS} UnrealEditor-Linux-Development
```

**Gating:** `GenerateEngineDockerfile()` accepts a new `MacOSHost bool` field in `DockerfileOptions`. When true, generates the modified Stage 3 and Linux-explicit Stage 4.

---

### 4. `--platform` Flag for Engine Image Build

**Change:** `EngineImageBuilder.Build()` passes `--platform linux/<arch>` to the container build command, where `<arch>` comes from `EngineImageOptions.Arch` (defaulting to the host's `game.arch` config).

Currently the engine image builder omits `--platform` (the game container builder already passes it correctly). This means on macOS, without `--platform`, the image defaults to the host's native architecture — which would be `linux/arm64` on Apple Silicon, correct for `game.arch=arm64` but wrong if the user wants `amd64`. Making it explicit ensures correctness regardless of Docker/Podman daemon defaults.

---

### 5. `.dockerignore` Addition

Add to `baseDockerignore()`:

```
# Mac-platform dotnet — not usable inside Linux containers
Engine/Binaries/ThirdParty/DotNet/mac-arm64/
Engine/Binaries/ThirdParty/DotNet/mac-x64/
```

This shrinks the build context by ~200 MB and prevents the wrong dotnet from appearing inside the container.

---

### 6. `ludus engine setup` macOS-Aware Path

`ludus engine setup` currently runs `Setup.sh` on the host (macOS), downloading the macOS toolchain. On macOS with a container backend configured, it should additionally run the Linux toolchain bootstrap pre-flight (Component 1 above).

**Behavior change for `ludus engine setup` on macOS + container backend:**
1. Run `Setup.sh` on host (existing behavior — macOS toolchain)
2. Run Linux toolchain bootstrap container (new — Linux toolchain)
3. Run GenerateProjectFiles bootstrap container (new — Linux Makefile)

Users who run `ludus engine setup` before `ludus engine build` get a fully pre-warmed state with zero surprises.

---

### 7. Prerequisite Check: macOS Container Build Readiness

Add a new check to `checkEngineSource()` (or a new `checkMacOSContainerBuild()`) that runs on macOS + container backend:

- Warns if Linux toolchain is missing (will be fetched automatically, but surfaced in `ludus init`)
- Warns if `Engine/Binaries/ThirdParty/DotNet/linux-arm64/` or `linux-x64/` is absent (means Setup.sh hasn't run in Linux yet — the bootstrap will handle it)

This gives users a clear `ludus init` diagnostic before they attempt a build.

---

## Data Flow

```
ludus engine build --backend podman  (macOS host)
  │
  ├─ [Pre-flight 1] Linux toolchain present?
  │     No  → podman run --rm --platform linux/<arch> -v <src>:/engine <base> bash Setup.sh
  │               → downloads Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/
  │     Yes → skip (logged at --verbose)
  │
  ├─ [Pre-flight 2] Engine source hash changed?
  │     Yes → podman run --rm --platform linux/<arch> -v <src>:/engine <base> bash GenerateProjectFiles.sh -makefile
  │               → writes Linux-targeted Makefile to engine root
  │     No  → skip
  │
  ├─ Write Dockerfile (MacOSHost=true variant)
  │     Stage 3 = no-op RUN
  │     Stage 4 = explicit Linux targets
  │     --platform linux/<arch> passed to build command
  │
  └─ podman build --platform linux/<arch> -t ludus-engine:<ver> -f <df> <src>
```

---

## File Map

| File | Change |
|------|--------|
| `internal/dockerbuild/macos_preflight.go` | New — `runLinuxToolchainBootstrap()`, `runGenerateProjectFiles()`, `MacOSPreflightNeeded()`, `LinuxToolchainPresent()` |
| `internal/dockerbuild/dockerfile.go` | Add `MacOSHost bool` to `DockerfileOptions`; modify Stage 3 and Stage 4 when set; add dotnet `.dockerignore` entries |
| `internal/dockerbuild/engine.go` | Pass `--platform linux/<arch>` in build args; call pre-flight functions before `writeBuildContext` when on macOS |
| `internal/dockerbuild/deps.go` | No change |
| `internal/engine/builder_unix.go` | No change — native builder path; pre-flight is container-path only |
| `cmd/engine/engine.go` | Pass `Arch` through to `EngineImageOptions`; wire `ludus engine setup` macOS pre-flight into `runSetup()` when container backend is configured |
| `internal/prereq/checker_docker.go` | Add `checkMacOSContainerBuild()` check |
| `internal/config/config_types.go` | No change — `engine.backend` and `game.arch` already carry what we need |
| `internal/toolchain/toolchain.go` | Add `LinuxToolchainPresentForMacOS()` helper that checks the HostLinux SDK path |

---

## Testing Strategy

### Unit tests
- `TestLinuxToolchainPresent_*` — verify detection of present/absent toolchain at various paths
- `TestDockerfileOptions_MacOSHost` — verify Stage 3 is no-op and Stage 4 uses Linux targets
- `TestDockerignore_ExcludesMacDotNet` — verify mac-arm64/mac-x64 DotNet entries present
- `TestMacOSPreflightNeeded_*` — cache hit/miss logic for GenerateProjectFiles hash

### Integration behavior (CI-verified on Linux/Windows, manually on macOS)
- Linux host: pre-flight code path never entered, Dockerfile unchanged
- Windows host: pre-flight code path never entered, Dockerfile unchanged
- macOS + native backend: pre-flight not triggered (container backend check)
- macOS + podman/docker backend: pre-flight runs, Dockerfile uses macOS variant

### CI (Linux/Windows)
All existing tests pass unmodified. New unit tests cover the macOS-gated helpers in isolation (no real container invoked in tests — pre-flight functions accept an injectable runner for testability).

---

## Backward Compatibility

- All changes gated on `darwin` + container backend — zero impact on Linux and Windows
- No new required config fields — behavior is automatic
- `ludus.yaml` gains no new fields; `--skip-setup` and `--skip-generate` flags may be added as escape hatches but are not required for the core feature
- Engine image tags are unchanged; the resulting image is functionally identical to one built on Linux

---

## Out of Scope

- Native macOS engine build (`--backend native`) — Setup.sh on macOS builds for Mac, which is correct for native use
- macOS game client builds — separate concern, not blocked by this
- Windows Subsystem for Linux on macOS — not a real thing; not considered
