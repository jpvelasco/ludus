# Ludus

A CLI tool that automates the end-to-end pipeline for deploying Unreal Engine 5 dedicated servers to AWS GameLift Containers.

Ludus handles the entire workflow that would otherwise require dozens of manual steps across multiple tools: UE5 source builds, game server compilation, Docker containerization, ECR push, and GameLift fleet deployment. While Lyra (Epic's sample game) is the default project, Ludus supports any UE5 game with dedicated server targets.

## What it does

```
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

- **OS**: Linux x86_64 (Ubuntu recommended)
- **RAM**: 16 GB minimum (UE5 linking uses ~8 GB per job)
- **Disk**: 100 GB free (after engine source is on disk)
- **Go**: 1.23.5+

### External tools

- **Docker** — for container image builds
- **AWS CLI v2** — configured with credentials (`aws configure sso` or standard config)
- **Git** — for engine source management

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
   ```
   <engine>/Samples/Games/Lyra/Content/
   ```
6. Also copy any plugin `Content/` folders if present

> **Note**: This is the most friction-heavy step. The Epic Games Launcher does not run on Linux, so Linux developers need access to a Windows or macOS machine for this one-time download.

### AWS setup

- An AWS account with permissions for GameLift, ECR, IAM, and STS
- Configure authentication: `aws configure sso` or set `AWS_PROFILE`
- An ECR repository (Ludus will push container images here)

## Installation

```bash
git clone git@github.com:jpvelasco/ludus.git
cd ludus
go build -o ludus -v
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
| `game.projectName` | UE5 project name | `Lyra` |
| `game.serverMap` | Default server map | `L_Expanse` |
| `container.serverPort` | Game server UDP port | `7777` |
| `gamelift.instanceType` | EC2 instance type for fleet | `c6i.large` |
| `aws.region` | AWS region | `us-east-1` |
| `aws.accountId` | AWS account ID (for ECR URI) | (required) |

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
# Validate prerequisites
./ludus init --verbose

# Build engine only
./ludus engine build --verbose

# Build game server only
./ludus game build --verbose

# Build and push container
./ludus container build --verbose
./ludus container push --verbose

# Deploy to GameLift
./ludus deploy fleet --verbose

# Tear down all Ludus-managed AWS resources
./ludus deploy destroy --verbose
```

### Global flags

| Flag | Description |
|------|-------------|
| `--verbose` / `-v` | Show detailed output including shell commands |
| `--dry-run` | Print commands without executing |
| `--json` | Output in JSON format |
| `--config <path>` | Config file path (default: `./ludus.yaml`) |

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

- **NuGet audit errors** — UE 5.6's Gauntlet test framework depends on Magick.NET 14.7.0 which has known CVEs. Combined with Epic's `TreatWarningsAsErrors`, this breaks AutomationTool compilation. Ludus writes a `Directory.Build.props` setting `NuGetAuditLevel=critical` to allow non-critical CVEs through while still catching critical ones.

- **Multiple server targets** — UE 5.6 Lyra ships with 4 server targets (LyraServer, LyraServerEOS, LyraServerSteam, LyraServerSteamEOS). RunUAT refuses to build without `DefaultServerTarget` configured. Ludus sets this automatically in `DefaultEngine.ini`.

- **Engine make targets** — `LyraServer` is built via RunUAT (stage 3), not via `make` during engine compilation (stage 2). Ludus only builds `ShaderCompileWorker` and `UnrealEditor` in the engine stage.

## Resource management

Ludus tags all AWS resources it creates with:
- `ludus:managed = true`
- `ludus:fleet-name = <fleet-name>`

Use `ludus deploy destroy` to tear down all Ludus-managed resources in reverse order (fleet, container group definition, IAM role). Resources that don't exist are skipped gracefully.

## Roadmap

### Near-term

- ~~**Pluggable deployment targets**~~ (done) — `deploy.Target` interface with `gamelift` and `binary` implementations. Pipeline stages gated by target capabilities. `--target` flag on deploy subcommands. Future targets (Agones, Hathora) implement the same interface.
- **Cross-compile toolchain management** — Auto-detect and download the correct Linux cross-compile toolchain for the target UE version (clang-18 for 5.6, clang-20 for 5.7).

### Mid-term

- **AI agent orchestration (MCP)** — Ludus's CLI architecture (discrete idempotent commands, `--json` output, `--dry-run`) makes it a natural execution layer for AI agents. The agent handles non-deterministic decisions (build failure diagnosis, recovery, instance type optimization), while Ludus handles deterministic execution. A separate MCP wrapper would expose Ludus commands as tools for Claude, GPT, or other agents.
- **GitHub Actions / CI integration** — Generate CI workflow files (`ludus ci init`) for GitHub Actions, GitLab CI, or shell scripts. Epic's EULA blocks distributing pre-built engine images, so CI requires self-hosted runners — Ludus generates the workflow that assumes this setup.
- **Build caching** — Skip unchanged pipeline stages based on file hashes. Track build artifacts and skip engine/cook stages when inputs haven't changed.

### Long-term

- **BuildGraph / DAG-based orchestration** — Define build steps as a directed acyclic graph. Enables parallel builds, distributed execution, artifact caching, and pluggable VCS support (Git, Perforce, Plastic SCM).
- **WSL2 support** — OS prereq check update, `.wslconfig` memory guidance, Linux filesystem for I/O performance.
- **macOS support** — Mac-specific engine scripts, cross-compilation strategy.

## License

[MIT](LICENSE)
