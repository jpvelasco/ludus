<p align="center">
  <img src="docs/logo.png" alt="Ludus" width="200">
</p>

<p align="center">
  <a href="https://www.npmjs.com/package/ludus-cli"><img src="https://img.shields.io/npm/dw/ludus-cli?style=flat-square&logo=npm" alt="npm downloads"></a> <a href="https://github.com/jpvelasco/ludus/releases/latest"><img src="https://img.shields.io/github/v/release/jpvelasco/ludus?style=flat-square" alt="Latest Release"></a> <a href="https://goreportcard.com/report/github.com/jpvelasco/ludus"><img src="https://goreportcard.com/badge/github.com/jpvelasco/ludus?style=flat-square" alt="Go Report Card"></a> <a href="https://app.codecov.io/gh/jpvelasco/ludus"><img src="https://img.shields.io/codecov/c/github/jpvelasco/ludus?style=flat-square&logo=codecov" alt="Codecov"></a> <a href="https://app.codacy.com/gh/jpvelasco/ludus"><img src="https://app.codacy.com/project/badge/Grade/2abf7453cf2e462eb3d0c5454a3ecf33" alt="Codacy"></a> <a href="https://github.com/jpvelasco/ludus/blob/main/LICENSE"><img src="https://img.shields.io/github/license/jpvelasco/ludus?style=flat-square" alt="License"></a>
</p>

# Ludus

**The fastest way to build, cook, and deploy Unreal Engine 5 dedicated servers.**

One command. Multiple backends. Production-ready GameLift, EC2, or binary output.

```bash
# Full pipeline in one command
ludus run
```

Now with official UE 5.8 support, Zen DDC as default, OpenTelemetry observability, and AWS Account ID masking.

---

A CLI tool that automates the end-to-end pipeline for deploying Unreal Engine 5 dedicated servers to AWS GameLift.

Ludus handles the entire workflow that would otherwise require dozens of manual steps across multiple tools: UE5 source builds, game server compilation, Docker containerization, ECR push, and GameLift fleet deployment. For local development, GameLift Anywhere mode skips containers entirely — fleet creation takes seconds instead of minutes. While Lyra (Epic's sample game) is the default project, Ludus supports any UE5 game with dedicated server targets.

## Quickstart

```bash
# Install
npm install -g ludus-cli

# Configure (edit ludus.yaml with your engine path and AWS settings)
ludus setup

# Validate your environment
ludus init --verbose

# Run the full pipeline
ludus run --verbose
```

**Prerequisites at a glance**: UE5 source build, Docker/Podman, AWS CLI v2, Go 1.24+, 16 GB RAM, 300 GB disk (native) / 2 TB disk (container builds recommended). macOS (Apple Silicon/Intel) supported via container backends only. See [detailed prerequisites](#prerequisites) below.

## What it does

```bash
ludus run --verbose
```

This single command orchestrates six stages:

1. **Prerequisite validation** — OS, engine source, game content, Docker, AWS CLI, disk space, RAM
2. **Engine build** — UE5 source compilation (Setup.sh, project files, make)
3. **Game server build** — Dedicated server packaging via RunUAT BuildCookRun
4. **Container build** — Dockerfile generation and Docker image build
5. **ECR push** — Docker image push to Amazon ECR
6. **GameLift deploy** — Container fleet creation with IAM roles and polling

## Prerequisites

### System requirements

- **OS**: Windows 10/11, Linux x86_64 (Ubuntu recommended), or macOS (Apple Silicon/Intel via `--backend docker` or `--backend podman` only; see macOS subsection below)
- **RAM**: 16 GB minimum (UE5 linking uses ~8 GB per job)
- **Disk**: 300 GB free for native builds. **2 TB recommended for container builds** (1 TB minimum) — the multi-stage Docker build accumulates ~200 GB of BuildKit layer cache on top of the 60–100 GB engine image, UE source, and game build artifacts. If running both an engine build and a game build on the same host, budget extra headroom. Run `docker builder prune -af` between pipeline stages to reclaim cache. Note: `ludus init` only validates a 300 GB free-disk floor and does not yet enforce the larger container-build requirement, so a host with 300–999 GB free will pass validation but can still run out of disk mid-build.
- **Go**: 1.24+

### macOS (container backends)
macOS requires a container backend (`--backend docker` or `--backend podman`; install Docker Desktop or Podman Desktop). Native engine builds target macOS, not Linux. Engine container builds always use `linux/amd64` (QEMU emulation; Epic ships only x86_64 Linux toolchain). Use pre-built engine image in `ludus.yaml` (`engine.dockerImage`) to skip QEMU. Run `ludus doctor` for checks. See [macOS Support](#macos-support) for examples and Graviton (`--arch arm64`) workflow.

### External tools

- **Docker** or **Podman** --- for container image builds (Podman recommended on Windows for large engine images; see [Docker Desktop vs Podman](#docker-desktop-vs-podman-on-windows))
- **AWS CLI v2** --- configured with credentials (`aws configure sso` or standard config)
- **Git** --- for engine source management

### Unreal Engine 5 (source build)

UE5 must be built from source — Epic Launcher builds cannot produce dedicated server targets.

1. Get access to the [UE5 source on GitHub](https://www.unrealengine.com/en-US/ue-on-github) (requires Epic Games account linked to GitHub)
2. Clone the engine source:
   ```bash
   git clone https://github.com/EpicGames/UnrealEngine.git -b 5.6.1-release UnrealEngine-5.6.1-release
   ```

### Lyra Content (manual download required)

Epic does not include Lyra game assets in the GitHub source. The `Content/` folder must be downloaded separately from the Epic Games Launcher Marketplace.

1. Install the [Epic Games Launcher](https://www.unrealengine.com/en-US/download) on Windows or macOS (not available on Linux)
2. Install UE 5.6 through the launcher (version must match your source build)
3. Add [Lyra Starter Game](https://www.fab.com/listings/93faede1-4434-47c0-85f1-bf27c0820ad0) from Fab to your library
4. Create a project from it — this downloads the content assets
5. Copy the `Content/` folder to your engine source tree:
   ```text
   <engine>/Samples/Games/Lyra/Content/
   ```
6. Also copy any plugin `Content/` folders if present

> **Note**: This is the most friction-heavy step. The Epic Games Launcher does not run on Linux, so Linux developers need access to a Windows or macOS machine for this one-time download.

### AWS setup

- An AWS account with permissions for GameLift, ECR, IAM, and STS
- Configure authentication: `aws configure sso` or set `AWS_PROFILE`
- An ECR repository (Ludus will push container images here). `ludus container push` auto-creates the repository if it's missing, which needs the `ecr:CreateRepository` action (not included in `AmazonEC2ContainerRegistryPowerUser`). For least-privilege/CI roles that only push, pre-create the repository (`aws ecr create-repository --repository-name <repo>`) and grant push/pull only.

## Installation

### Via npm (recommended)

```bash
npm install -g ludus-cli
```

Or run directly without installing:

```bash
npx ludus-cli --help
```

### From source

```bash
git clone git@github.com:jpvelasco/ludus.git
cd ludus
go build -o ludus -v .
```

## Configuration

```bash
cp ludus.example.yaml ludus.yaml
```

Edit `ludus.yaml` with your environment settings. Key fields:

| Setting | Description | Default |
|---------|-------------|---------|
| `engine.sourcePath` | Path to UE5 source directory | (required) |
| `engine.maxJobs` | Max parallel compile jobs (0 = auto-detect from RAM) | `0` |
| `engine.backend` | Build backend: `native`, `docker`, `podman`, or `wsl2` | `native` |
| `engine.dockerImage` | Pre-built engine Docker image URI (skips engine build) | (empty) |
| `engine.dockerImageName` | Local Docker image name for engine builds | `ludus-engine` |
| `engine.dockerBaseImage` | Base Docker image for engine builds | `ubuntu:22.04` |
| `game.projectName` | Convenience label, used **only** to default the target names (`<projectName>Server`, etc.). Not used for packaged/container paths and need not match the `.uproject` filename. | `Lyra` |
| `game.projectPath` | Full path to the `.uproject` file — must include the filename (e.g. `/home/user/MyGame/MyGame.uproject`). **Source of truth for paths:** UE names the packaged content dir after the `.uproject` filename, so staged/container paths derive from this, not from `projectName`. | (empty = auto-detect Lyra) |
| `game.serverTarget` | Server build target name — the binary name without "Target" suffix (e.g. `LyraServer`, not `LyraServerTarget`). Set explicitly when it differs from `<projectName>Server`. | `<projectName>Server` |
| `game.serverMap` | Default server map | `L_Expanse` |
| `container.serverPort` | Game server UDP port | `7777` |
| `game.arch` | Target architecture: `amd64` or `arm64` (Graviton) | `amd64` |
| `deploy.target` | Deployment target: `gamelift`, `stack`, `ec2`, `binary`, `anywhere` | `gamelift` |
| `gamelift.instanceType` | EC2 instance type for fleet | `c6i.large` |
| `anywhere.locationName` | Custom location name for Anywhere fleet | `custom-ludus-dev` |
| `aws.region` | AWS region | `us-east-1` |
| `aws.accountId` | AWS account ID (for ECR URI) | (required for container targets) |
| `ddc.mode` | Derived Data Cache mode: `zen` (Zen Store, default), `local` (legacy FileSystem cache, deprecated), or `none` (disabled) | `zen` |
| `ddc.zenPath` | Host path for the Zen Store DDC — persists the cook cache across container runs | `~/.ludus/zen` |
| `ddc.localPath` | Host path for the legacy FileSystem DDC (mode `local` only) | `~/.ludus/ddc` |
| `observability.logs.enabled` | Persist build output to per-run log files | `true` |
| `observability.logs.dir` | Build log directory (project-local) | `.ludus/logs` |
| `observability.logs.retainRuns` | Number of run logs to keep before pruning oldest | `20` |
| `observability.otlp.enabled` | Export per-stage build spans via OpenTelemetry (OTLP) | `false` |
| `observability.otlp.endpoint` | OTLP collector endpoint (host:port) | (empty) |
| `privacy.maskAccountId` | Mask the AWS account ID in ECR URIs/ARNs in terminal output (JSON/MCP unaffected). Override per-run with `--show-account-id` | `true` |

See [`internal/config/config.go`](internal/config/config.go) for the full list of configuration keys including CI, EC2 fleet, and content validation options.

## Usage

### Full pipeline

```bash
# Run all 6 stages
./ludus run --verbose

# Skip engine build (use existing)
./ludus run --verbose --skip-engine

# Skip game build (use existing packaged server)
./ludus run --verbose --skip-engine --skip-game

# Dry run — print commands without executing
./ludus run --dry-run
```

### Individual commands

```bash
# Interactive first-run setup wizard
./ludus setup

# Validate prerequisites (--fix to auto-remediate)
./ludus init --verbose

# Deep diagnostics (toolchain, disk, Docker, AWS, security lint)
./ludus doctor

# Build engine only
./ludus engine build --verbose

# Build game server only (--arch arm64 for Graviton)
./ludus game build --verbose

# Build and push container
./ludus container build --verbose
./ludus container push --verbose

# Deploy to GameLift (imperative API calls)
./ludus deploy fleet --verbose

# Deploy via CloudFormation (atomic with rollback)
./ludus deploy stack --verbose

# Deploy via Managed EC2 (no Docker required)
./ludus deploy ec2 --verbose

# Deploy locally via GameLift Anywhere (seconds, not minutes)
./ludus deploy anywhere --verbose

# Create a game session and connect
./ludus deploy session
./ludus connect

# Tear down all Ludus-managed AWS resources
./ludus deploy destroy --verbose

# Generate BuildGraph XML for Horde/UET
./ludus buildgraph -o build.xml

# DDC management
./ludus ddc status
./ludus ddc warmup --verbose
./ludus ddc clean
./ludus ddc prune --days 30

# Build logs (build output is persisted to .ludus/logs/ by default)
./ludus logs list                # list recent build runs
./ludus logs path                # print the latest run's log path
./ludus logs tail                # tail the latest run's log

# Quick config changes
./ludus config set game.arch arm64
./ludus config get engine.sourcePath
```

### Container build backend

Instead of building the engine natively on the host, Ludus can build UE5 inside a container (Docker or Podman), producing a reusable image. CI runners then pull the image to run game builds without recompiling the engine.

```bash
# Build engine inside a container (produces a reusable image)
./ludus engine build --backend docker --verbose   # or --backend podman

# Push the engine image to ECR
./ludus engine push --verbose

# Build game server using the engine image
./ludus game build --backend docker --verbose

# Full pipeline with container backend
./ludus run --backend docker --verbose
```

The `--backend` flag can be set per-command or configured as a default in `ludus.yaml`:

```yaml
engine:
  backend: "docker"   # or "podman"
```

**Pre-built engine image**: If the engine image already exists in a registry (built once by a team member or CI), point to it directly and skip the engine build entirely:

```yaml
engine:
  backend: "docker"
  dockerImage: "123456789.dkr.ecr.us-east-1.amazonaws.com/ludus-engine:5.6.1"
```

With `dockerImage` set, `ludus game build --backend docker` and `ludus run --backend docker` will skip the engine build stage and use the specified image for game builds.

**How it works**:
- `ludus engine build --backend docker` generates a Dockerfile (configurable base image, default Ubuntu 22.04), runs `docker build` (or `podman build`) with the engine source as context, and tags the image as `ludus-engine:<version>`. Use `--base-image` or set `engine.dockerBaseImage` in `ludus.yaml` to use Amazon Linux, RHEL, or other bases (auto-detects `apt-get` vs `dnf`)
- `ludus engine push` authenticates with ECR and pushes the image (creates the ECR repository if needed)
- `ludus game build --backend docker` runs `docker run` (or `podman run`) with volume mounts for the packaged output, executing RunUAT BuildCookRun inside the engine container
- The rest of the pipeline (container build, ECR push, deploy) works unchanged --- the game server output directory is the same regardless of backend

**Notes**:
- Engine container images are large (60-100 GB) --- this is expected for UE5. Use `--skip-engine` to produce smaller images from pre-built binaries.
- Container client builds are Linux-only (Win64 cross-compile in containers is not supported)
- Epic's EULA allows private engine images within an organization; the restriction is on public distribution

### Docker Desktop vs Podman on Windows

Docker Desktop's containerd storage backend has a lease timeout that crashes during image export for large UE5 engine images (60-100+ GB). The engine compiles successfully but the final image export phase fails with errors like `lease does not exist` or `failed to solve: exporting to image`. This is a [known Docker Desktop limitation](https://github.com/docker/for-win/issues) with no workaround.

**Podman** uses its own `containers/storage` driver without lease timeouts, making it a reliable alternative for large image builds. All Ludus commands accept `--backend podman` as a drop-in replacement for `--backend docker`.

> **Note**: GameLift container image builds (`ludus container build`) and ECR pushes (`ludus container push`, `ludus engine push`) currently use Docker only. These images are small (~3-5 GB) and are unaffected by the lease timeout issue. Podman support for GameLift containers is planned for a future release.

#### Installing Podman on Windows

Install Podman Desktop or the CLI:

```bash
# Option 1: Install via winget (recommended)
winget install RedHat.Podman

# Option 2: Download the installer from https://podman.io/docs/installation#windows
```

Then initialize the Podman machine (a lightweight WSL2 VM):

```bash
# Create and start the machine
podman machine init
podman machine start

# Verify installation
podman --version
podman machine list
```

On Linux, Podman runs natively without a machine --- just install via your package manager (`apt install podman`, `dnf install podman`, etc.).

#### Using Podman with Ludus

```bash
# Package pre-built engine binaries into a container image
ludus engine build --backend podman --skip-engine

# Full pipeline: build game server + deploy with persistent DDC
ludus run --backend podman --ddc zen
```

These two commands are the recommended workflow. `--skip-engine` packages your existing Linux binaries into the image without recompiling (minutes, not hours). `--ddc zen` enables persistent shader caching so subsequent builds skip expensive re-derivation.

Other useful commands:

```bash
# Build game server only (no deploy)
ludus game build --backend podman --ddc zen --verbose

# Build engine from source inside Podman (full compile, slow)
ludus engine build --backend podman --verbose
```

Or set Podman as the default backend in `ludus.yaml`:

```yaml
engine:
  backend: "podman"
```

#### Recommended workflow for Windows

Build the engine natively on the host, then package the pre-built Linux binaries into a Podman image with `--skip-engine`. This avoids both multi-hour recompilation inside the container and Docker Desktop's export crashes:

```bash
# 1. Package pre-built binaries into container image
ludus engine build --backend podman --skip-engine

# 2. Build and deploy with persistent DDC
ludus run --backend podman --ddc zen
```

The `--skip-engine` flag generates a lean 2-stage Dockerfile that copies pre-built binaries directly from the host instead of compiling inside the container. Combined with `--ddc zen` for persistent shader caching, this is the fastest iteration path on Windows.

**Image size trade-off**: UE5 engine images are large (60-100+ GB) because they include the full editor, shader compiler, build tools, and runtime libraries needed for BuildCookRun. The runtime stage also installs X11, accessibility, and audio libraries (~150 MB) that UnrealEditor-Cmd links against even in headless/server mode. This is inherent to UE5's architecture and applies to both Docker and Podman. Use `.dockerignore` (generated automatically by Ludus) to exclude host-platform binaries, debug symbols, and build intermediates from the build context.

### WSL2 build backend (Windows)

On Windows, Ludus can build the engine and game server directly inside a WSL2 Linux distro, bypassing container runtimes entirely. This gives native Linux I/O performance without Docker or Podman overhead.

Two modes:

- **Default** (`--backend wsl2`): Zero setup. Accesses your engine source via `/mnt/<drive>/` (virtiofs). Works immediately but I/O is slower for large codebases.
- **High-performance** (`--backend wsl2 --wsl-native`): One-time rsync of engine source to native ext4 inside WSL2 (`~/ludus/engine/<version>/`). DDC cache also lives on ext4. 3-10x faster I/O for builds and cooking.

```bash
# Build engine in WSL2 (zero-setup, uses /mnt/ virtiofs)
ludus engine build --backend wsl2 --verbose

# Build engine with native ext4 for maximum speed (one-time rsync)
ludus engine build --backend wsl2 --wsl-native --verbose

# Build game server in WSL2 with persistent DDC cache
ludus game build --backend wsl2 --ddc zen --verbose

# Full pipeline with WSL2 backend
ludus run --backend wsl2 --verbose

# Full pipeline with native ext4 sync for fastest builds
ludus run --backend wsl2 --wsl-native --verbose
```

**Options**:

| Flag | Description |
|------|-------------|
| `--backend wsl2` | Use WSL2 instead of native/container build |
| `--wsl-native` | Rsync source to native ext4 (3-10x faster I/O, requires ~120 GB free) |
| `--wsl-distro <name>` | Target a specific distro (default: first running WSL2 distro) |
| `--ddc zen` | Persistent Zen Store DDC cache (default) — works on both virtiofs and native ext4 |

Build dependencies (`gcc`, `make`, `cmake`, `python3`) are installed automatically on first run. If WSL2 is not available, Ludus recommends Podman as a fallback.

Or set WSL2 as the default backend in `ludus.yaml`:

```yaml
engine:
  backend: "wsl2"
```

### macOS Support

Ludus fully supports macOS (Apple Silicon and Intel) using container backends (`--backend docker` or `--backend podman`). This is the primary supported path for producing Linux dedicated servers from a Mac.

Native engine builds on macOS target macOS, not Linux. Container builds use Linux base images and the official Linux toolchain inside the container.

#### Recommended workflow for Apple Silicon users targeting Graviton

On Apple Silicon (`darwin/arm64`):

- **Engine container builds** always target `linux/amd64`. This is required because Epic only ships an x86_64 Linux toolchain. The build runs under QEMU user-mode emulation. **This is impractical for production use**: expect 8–12× slower compile times versus native x86_64 Linux or WSL2 — a full engine build that takes 90 minutes on Linux can take 12+ hours under QEMU. Use a pre-built engine image (see below) to avoid this entirely.

- **Game builds** with `--arch arm64` (for Graviton) cross-compile inside the emulated amd64 environment. The resulting `LinuxArm64Server` binaries are correct and deploy to Graviton instances.

- The engine image itself stays amd64 even if you later build an arm64 game container image.

To avoid the QEMU cost for engine builds on your Mac, build the engine image once on a native x86_64 Linux machine or CI runner, push to a registry, and point at the pre-built image:

```yaml
engine:
  backend: "docker"
  dockerImage: "123456789.dkr.ecr.us-east-1.amazonaws.com/ludus-engine:5.6.1"
```

Subsequent game builds and runs will skip the engine stage entirely.

#### Common one-command examples

```bash
# Build engine inside container (QEMU on Apple Silicon)
./ludus engine build --backend docker --verbose

# Same with Podman
./ludus engine build --backend podman --verbose

# Build game server for Graviton (cross-compile inside the amd64 container)
./ludus game build --arch arm64 --backend docker --verbose

# Build the final container image for the packaged server
./ludus container build --verbose
./ludus container push --verbose

# Full pipeline targeting arm64/Graviton
./ludus run --backend docker --arch arm64 --verbose
```

Set sensible defaults in `ludus.yaml`:

```yaml
engine:
  backend: "docker"
game:
  arch: "arm64"
```

See the [ARM64 / Graviton workflow](#arm64--graviton-workflow) for deployment commands (e.g. `ludus deploy fleet --with-session` automatically selects Graviton instance types).

Run `ludus doctor` for macOS + container environment checks.

### DDC Zen Support (Default)

Ludus uses **Zen** — Unreal Engine's modern high-performance Derived Data Cache backend — as the **default** for all supported UE versions (5.4+).

```yaml
ddc:
  mode: "zen"            # Default: Zen (recommended)
  zenPath: "~/.ludus/zen" # Persistent Zen Store location
```

**Benefits:**

- Cut warm-cache cook times by 40–70%
- Keep derived data cached across local, WSL2, Docker, and Podman builds
- Mount the Zen Store into containers automatically

Legacy `localPath` (Filesystem DDC) is still available but deprecated.

- `--ddc zen` (default) — Persists UE's Zen Store cook cache. For container builds (Docker/Podman), the host Zen directory (`~/.ludus/zen`) is mounted into the container so the cache survives `--rm`. For native and WSL2 builds, UE's autolaunched Zen Store already persists in your home directory, so Ludus leaves it untouched.
- `--ddc local` (deprecated) — Legacy FileSystem cache on the host (`~/.ludus/ddc`), redirected via `UE-LocalDataCachePath`. Retained for edge cases; prefer `zen`.
- `--ddc none` — Disable DDC (useful for clean CI runs)
- Docker and Podman build identically — DDC behaves the same on both.
- `ludus ddc` subcommands: `status`, `clean`, `prune`, `warmup`

> **Note:** `ludus ddc clean`/`prune`/`status` manage the Ludus-owned cache directories (the Zen mount for container builds, the FileSystem cache for `local`). For native/WSL2 `zen` builds, UE owns the cache location in your home directory, so those subcommands don't apply there.

The `ludus ddc warmup` command pre-populates engine-level data so even the first cook after enabling DDC is noticeably faster.

```bash
# Check DDC status
./ludus ddc status

# Pre-warm the cache (cook-only Docker build)
./ludus ddc warmup --verbose

# Clean the entire cache
./ludus ddc clean

# Remove entries older than 30 days
./ludus ddc prune --days 30

# Disable DDC for a single build
./ludus game build --ddc none --verbose
```

Configure DDC in `ludus.yaml`:

```yaml
ddc:
  mode: "zen"             # "zen" (default), "local" (legacy FileSystem cache), or "none"
  zenPath: "~/.ludus/zen" # Persistent Zen Store host path
  localPath: ""           # Legacy FileSystem path, mode "local" only (default: ~/.ludus/ddc)
```

#### DDC Performance — Up to 59% Faster Cooks on WSL2 Native

Measured on WSL2 native ext4 (`--backend wsl2 --wsl-native`), Lyra sample project, UE 5.7.4, x86_64, with Zen cache fully wiped before the cold run:

| Phase | Cold (empty Zen cache) | Warm (cached) | Speedup |
|-------|------------------------|---------------|---------|
| Cook | 1321s (22m) | 541s (9m) | **59% faster** |
| Compile | 308s (5m) | 198s (3m) | 36% (incremental) |
| Stage | 482s (8m) | 346s (6m) | 28% |
| Archive | 83s | 68s | 18% |
| **BuildCookRun total** | **2205s (37m)** | **1160s (19m)** | **47%** |

The cook phase speedup (**59%**) is the DDC signal — warm Zen cache eliminates redundant shader compilation and asset derivation. Compile, stage, and archive phases also benefit from OS-level filesystem caching on native ext4.

Zen DDC cache size: ~330 MB after a full Lyra server cook.

Try it yourself:

```bash
ludus ddc clean
ludus game build --backend wsl2 --ddc zen --arch amd64
```

> **Note**: For native and WSL2 Zen builds, Unreal Engine owns the Zen Store under the user's home directory. Ludus manages `ddc.zenPath` for container builds, where it mounts the directory into the container so the cache persists.

**Recommended for best performance (Windows users):**

Download and build UE directly inside WSL2 to avoid virtiofs entirely:

```bash
# Inside WSL2
mkdir -p ~/ludus/engine
# Download/extract UE 5.7.4 directly into WSL2 (recommended)
ludus engine build --backend wsl2 --wsl-native
ludus game build --backend wsl2 --ddc zen
```

### Build caching

Ludus caches build results in `.ludus/cache.json` based on input hashes (git commit, config values, file metadata). If inputs haven't changed since the last successful build, the stage is skipped automatically.

```bash
# Normal run — unchanged stages are skipped via cache
./ludus run --verbose

# Force rebuild of all stages (ignore cache)
./ludus run --no-cache --verbose

# Force rebuild of a single stage
./ludus engine build --no-cache
./ludus game build --no-cache
./ludus container build --no-cache
```

Cache keys per stage:
- **Engine**: git HEAD of engine source, engine version, maxJobs, OS, backend, base image
- **Game server**: engine cache key + .uproject mtime/size, server target, game target, server map
- **Game client**: engine cache key + .uproject mtime/size, client target, platform
- **Container**: server build directory file manifest, project name, server target, server port, image tag

### Build Observability

Ludus includes structured logging to disk and optional OpenTelemetry (OTLP) export.

```yaml
observability:
  logs:
    enabled: true
    level: "info"
  otlp:
    enabled: false
    endpoint: "http://localhost:4318"
```

Logs are written to `.ludus/logs/` by default. Excellent for debugging complex builds and integrating with tools like Grafana, Jaeger, or Prometheus.

**On-disk logs.** Every run tees its stdout/stderr to a timestamped file under `.ludus/logs/` (project-local). This is on by default; the newest `observability.logs.retainRuns` files are kept and older ones pruned.

```bash
./ludus logs list                # list recent runs, newest first
./ludus logs path                # absolute path to the latest run's log
./ludus logs tail                # tail the latest run's log
./ludus run --no-logs            # disable logging for this run
```

Dry-run output is never written to disk.

**Distributed tracing (optional).** When enabled, Ludus exports one OpenTelemetry span per pipeline stage under a single `ludus.run` root span — useful for seeing where time goes across engine/game/container/deploy in a collector like Jaeger or Grafana Tempo. It is a no-op with zero overhead unless turned on, via config or the standard `OTEL_*` environment variables.

```yaml
observability:
  logs:
    enabled: true            # persist per-run logs (default: true)
    dir: ".ludus/logs"       # log directory (project-local)
    retainRuns: 20           # keep N newest runs, prune the rest
  otlp:
    enabled: false           # export per-stage traces (default: false)
    endpoint: "localhost:4318"   # OTLP/HTTP collector endpoint
    insecure: true           # plaintext (typical for a local collector)
```

### Global flags

| Flag | Description |
|------|-------------|
| `--verbose` / `-v` | Show detailed output including shell commands |
| `--dry-run` | Print commands without executing |
| `--json` | Output in JSON format |
| `--config <path>` | Config file path (default: `./ludus.yaml`) |
| `--profile <name>` | Use a named profile (isolates config and state) |
| `--ddc <mode>` | DDC mode: `zen` (default), `local` (legacy), or `none` (disable) |
| `--no-logs` | Do not write build output to `.ludus/logs` |

## Build time estimates

Measured on an 8-core Ryzen 7 2700X / 64 GB RAM / NVMe SSD (Windows, UE 5.6.1):

| Stage | Time | Notes |
|-------|------|-------|
| Engine build (from source) | ~3.5 hours | Full compile of ShaderCompileWorker + UnrealEditor; `maxJobs` auto-set to 8 |
| Lyra server build | ~45 min | RunUAT BuildCookRun: compile + cook (~3,900 packages) + stage + archive |
| Lyra client build (Win64) | ~45 min | Similar pipeline; incremental compile if engine is already built |
| Container build | ~5 min | Docker image from packaged server (~3 GB image) |
| ECR push | ~5–15 min | Depends on upload bandwidth |
| GameLift fleet deploy | ~10–20 min | Fleet creation + container download + activation polling |

**Full pipeline** (`ludus run`): roughly 5–6 hours on a first run. Subsequent runs with `--skip-engine` take under 2 hours.

These are ballpark figures. Actual times vary with CPU core count, RAM (affects max parallel jobs), disk speed, and network bandwidth. Content cooking is RAM-intensive — 32 GB recommended; on Ubuntu, disable `systemd-oomd` to prevent the OOM killer from terminating the cook process.

## Known issues and workarounds

Ludus automatically handles several UE5 5.6 build issues:

- **NuGet audit errors** — UE 5.6's Gauntlet test framework depends on Magick.NET 14.7.0 which has known CVEs. Combined with Epic's `TreatWarningsAsErrors`, this breaks AutomationTool compilation. Ludus sets `NuGetAuditLevel=critical` as an environment variable on RunUAT child processes (MSBuild reads env vars as properties), avoiding engine source modifications.

- **Multiple server targets** — UE 5.6 Lyra ships with 4 server targets (LyraServer, LyraServerEOS, LyraServerSteam, LyraServerSteamEOS). RunUAT refuses to build without `DefaultServerTarget` configured. Ludus sets this automatically in `DefaultEngine.ini`.

- **Engine make targets** — `LyraServer` is built via RunUAT (stage 3), not via `make` during engine compilation (stage 2). Ludus only builds `ShaderCompileWorker` and `UnrealEditor` in the engine stage.

## Resource management

Ludus tags all AWS resources it creates. Default tags (`ManagedBy: ludus`) are always applied, and custom tags can be configured in `ludus.yaml`:

```yaml
aws:
  tags:
    Project: "my-project"
    Environment: "dev"
```

The `Project` tag is auto-derived from `game.projectName` if not explicitly set.

**Deployment targets:**
- `ludus deploy fleet` — imperative API calls (container group definition → IAM role → fleet)
- `ludus deploy stack` — declarative CloudFormation stack (atomic, with automatic rollback on failure)
- `ludus deploy anywhere` — local development via GameLift Anywhere (see below)

#### Tearing down: `ludus deploy destroy`

By default, `ludus deploy destroy` is **scoped and safe** — it removes only the **ephemeral** resources for the active target and **preserves your durable build artifacts** (ECR repositories and S3 build buckets), which are expensive to recreate:

- `fleet` — fleet, container group definition, IAM role (reverse order)
- `stack` — the entire CloudFormation stack, atomically
- `ec2` — fleet, GameLift build, the uploaded S3 build **object**, IAM role
- `anywhere` — stops the server, deregisters compute, deletes fleet and location
- `binary` — the output directory

Two independent flags widen the scope:

| Flag | Effect |
| --- | --- |
| `--all-targets` | Tear down **every** target type, not just the active one. Still preserves durable artifacts. |
| `--purge` | **Also delete durable artifacts** — ECR repositories and S3 build buckets. Lists what will be deleted and prompts `[y/N]` first. |
| `--yes` / `-y` | Skip the `--purge` confirmation prompt (for CI/automation). |

```bash
./ludus deploy destroy                              # active target's fleet/group/IAM — ECR/S3 kept
./ludus deploy destroy --target anywhere            # a specific target
./ludus deploy destroy --all-targets                # sweep every target, artifacts kept
./ludus deploy destroy --purge                      # active target + ECR/S3 (prompts to confirm)
./ludus deploy destroy --all-targets --purge --yes  # full wipe, no prompt
```

> **Note:** `--purge` is irreversible — it deletes ECR images (your built engine/server containers) and S3 build buckets. The default `destroy` never touches them, so iterating on fleets is safe.

### GameLift Anywhere (local development)

GameLift Anywhere registers your local machine with GameLift so the game server runs locally while GameLift manages sessions, matchmaking, and player validation. Fleet creation takes seconds instead of 15–30 minutes for container fleets. No Docker build or ECR push required.

```bash
# Build the game server
./ludus game build --verbose

# Create Anywhere fleet + register machine + launch server
./ludus deploy anywhere --verbose

# Create a game session (works with existing session/connect commands)
./ludus deploy session

# Connect a client
./ludus connect

# Iterate: Ctrl+C the server, edit, rebuild, redeploy
./ludus deploy anywhere --verbose

# Clean up
./ludus deploy destroy --target anywhere
```

Use `--ip` to override the auto-detected local IP address. Configure defaults in `ludus.yaml`:

```yaml
anywhere:
  locationName: "custom-ludus-dev"  # Custom location name (must start with "custom-")
  ipAddress: ""                     # Leave empty to auto-detect
  awsProfile: "default"            # AWS profile for wrapper credentials
```

Anywhere is effectively free — AWS provides 3,000 sessions/month in the free tier.

## Deployment support matrix

Ludus supports five deployment targets with two build backends. Not every combination requires Docker, and ARM64 (Graviton) support varies by target.

### By deployment target

| Target | Command | Docker required? | ARM64 support | Best for |
|--------|---------|:---:|:---:|------|
| GameLift Containers | `deploy fleet` | Yes | Yes | Production container fleets |
| CloudFormation Stack | `deploy stack` | Yes | Yes | Production with atomic rollback |
| GameLift Managed EC2 | `deploy ec2` | No | Yes | Production without Docker |
| GameLift Anywhere | `deploy anywhere` | No | No (local only) | Local development/testing |
| Binary export | `deploy binary` | No | Yes | Custom deployment pipelines |

### How builds reach each target

```plaintext
game build --arch amd64|arm64
    |
    +--> container build + ECR push ---> deploy fleet
    |                                    deploy stack
    |
    +--> S3 upload (zip) -------------> deploy ec2
    |
    +--> File copy -------------------> deploy binary
                                        deploy anywhere
```

### ARM64 / Graviton workflow

ARM64 targets Graviton instances (20-30% cheaper than x86). The architecture flag flows through the entire pipeline.

> **macOS + container**: See the [macOS Support](#macos-support) section for the recommended Apple Silicon + Graviton workflow, QEMU emulation cost details, and copy-paste examples.

```bash
# Build ARM64 server (cross-compiles from Windows)
./ludus game build --arch arm64

# Option A: Container fleet (GameLift Containers)
./ludus container build --arch arm64    # docker build --platform linux/arm64
./ludus container push
./ludus deploy fleet --with-session     # auto-selects c7g.large Graviton instance

# Option B: Managed EC2 (no Docker needed)
./ludus deploy ec2 --arch arm64 --with-session
```

Set `game.arch: arm64` in `ludus.yaml` to default all commands to ARM64 without passing `--arch` each time.

## AI Agent Integration (MCP)

`ludus mcp` starts a [Model Context Protocol](https://modelcontextprotocol.io/) server over stdio, exposing the full pipeline as 26 tools. Any MCP-compatible AI agent — Claude Code, OpenCode, Claude Desktop, Kiro, Cursor, VS Code Copilot — can orchestrate builds, deployments, and game sessions programmatically.

### Prerequisites

- `ludus` binary built and available (on PATH or referenced by absolute path)
- `ludus.yaml` configured in the working directory
- Same external tools as CLI usage (Docker, AWS CLI, Git, Go)

### Client configuration

Add ludus as an MCP server in your agent's config. The JSON format varies by client.

#### Claude Code

```bash
claude mcp add ludus -- npx -y ludus-cli mcp
```

This registers ludus as a project-scoped MCP server. Restart Claude Code after adding.

#### OpenCode

Add to `opencode.json` in your project root (or `~/.config/opencode/config.json` globally):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "ludus": {
      "type": "local",
      "command": ["ludus", "mcp"],
      "enabled": true
    }
  }
}
```

#### Claude Desktop / Kiro / Cursor

These clients share the same format. Config file locations:

| Client | Config file |
|--------|------------|
| Claude Desktop | `%APPDATA%\Claude\claude_desktop_config.json` (Windows) / `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) |
| Kiro | `.kiro/settings/mcp.json` (workspace) or `~/.kiro/settings/mcp.json` (global) |
| Cursor | `.cursor/mcp.json` (workspace) or `~/.cursor/mcp.json` (global) |

```json
{
  "mcpServers": {
    "ludus": {
      "command": "ludus",
      "args": ["mcp"]
    }
  }
}
```

#### VS Code (Copilot)

Add to `.vscode/mcp.json` in your workspace:

```json
{
  "servers": {
    "ludus": {
      "command": "ludus",
      "args": ["mcp"]
    }
  }
}
```

### Available tools

| Domain | Tool | Description |
|--------|------|-------------|
| Init | `ludus_init` | Validate prerequisites (OS, engine source, toolchain, content, Docker, AWS, disk, RAM) |
| Status | `ludus_status` | Check status of all pipeline stages |
| Engine | `ludus_engine_setup` | Run Setup.sh to download engine dependencies |
| | `ludus_engine_build` | Build UE5 from source (long-running, blocks) |
| | `ludus_engine_push` | Push engine Docker image to ECR |
| Game | `ludus_game_build` | Build dedicated server via RunUAT (long-running, blocks) |
| | `ludus_game_client` | Build standalone client for Linux or Win64 (long-running, blocks) |
| Container | `ludus_container_build` | Generate Dockerfile and build container image |
| | `ludus_container_push` | Push container image to ECR |
| Deploy | `ludus_deploy_fleet` | Deploy GameLift container fleet (long-running) |
| | `ludus_deploy_stack` | Deploy via CloudFormation (long-running) |
| | `ludus_deploy_anywhere` | Deploy locally via GameLift Anywhere |
| | `ludus_deploy_ec2` | Deploy via GameLift Managed EC2 (no Docker) |
| | `ludus_deploy_session` | Create a game session, returns connection details |
| | `ludus_deploy_destroy` | Tear down the active target's ephemeral resources (durable ECR/S3 artifacts preserved). `all_targets` sweeps every target; `purge` also deletes durable artifacts |
| Connect | `ludus_connect_info` | Get connection info for current session and client build |
| BuildGraph | `ludus_buildgraph` | Generate BuildGraph XML for Horde/UET |
| DDC | `ludus_ddc_status` | Show DDC mode, path, and cache size on disk |
| | `ludus_ddc_clean` | Delete all DDC cache content, freeing disk space |
| | `ludus_ddc_configure` | Set DDC mode and/or path in ludus.yaml |
| | `ludus_ddc_warm` | Run a cook-only Docker build to pre-populate the DDC |
| Async | `ludus_engine_build_start` | Start engine build (returns immediately with build ID) |
| | `ludus_game_build_start` | Start game server build (returns immediately) |
| | `ludus_game_client_start` | Start client build (returns immediately) |
| | `ludus_build_status` | Poll build status, retrieve output, or cancel |

### Typical workflow

An agent orchestrating the full pipeline would call tools in this order:

```plaintext
ludus_init → ludus_engine_build → ludus_game_build → ludus_container_build →
ludus_container_push → ludus_deploy_fleet → ludus_deploy_session → ludus_connect_info
```

Use `ludus_status` to check which stages are already complete — agents can skip stages with cached results. For local development, replace the container/fleet steps with `ludus_deploy_anywhere`.

### Notes

- **Error handling**: Operational errors (build failures, AWS errors) return `CallToolResult` with `isError: true` and a JSON message. Go-level errors are reserved for protocol failures.
- **Async builds**: For long-running operations (engine/game builds), use the `_start` variants which return immediately with a build ID. Poll with `ludus_build_status` to check progress, retrieve output, or cancel. The synchronous tools (`ludus_engine_build`, `ludus_game_build`, `ludus_game_client`) block until complete.
- **Configuration**: All tools read from the same `ludus.yaml` as CLI commands. Every tool accepts `verbose` and `dryRun` parameters.

### Privacy

By default, Ludus **masks your AWS Account ID** in all human-readable terminal output (ECR URLs, ARNs, etc.) for safer screen sharing and video recording.

Override with:

```bash
ludus status --show-account-id
```

JSON output and MCP responses are never masked.

## Roadmap

- **WSL2 support** — OS prereq check update, `.wslconfig` memory guidance, Linux filesystem for I/O performance
- **macOS support** — Mac-specific engine scripts (Setup.command, Xcode), cross-compilation strategy

## License

Ludus is released under the **MIT License** (see [LICENSE](LICENSE) for full text).

All third-party dependencies are also MIT or Apache 2.0 licensed.

**Unreal Engine 5 usage note**:
This tool does **not** include or redistribute any UE5 source code or binaries.
You must obtain UE5 source code directly from Epic Games via GitHub (requires a valid Epic developer account). Ludus only orchestrates your legally obtained engine source and builds — all resulting engine images, game servers, and deployments are governed by Epic's EULA, which allows private use and modification but prohibits public distribution of built engine binaries.
