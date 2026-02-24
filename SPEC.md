# Ludus — Project Specification

> Latin for "game" / gladiator training school. A CLI tool that streamlines the
> end-to-end pipeline for deploying Unreal Engine 5 dedicated servers to
> AWS GameLift Containers. Supports any UE5 game with dedicated server targets
> (Lyra is the default sample project).

## Problem Statement

Getting an Unreal Engine 5 dedicated server built from source, containerized
for Linux, and deployed to AWS GameLift Containers is a multi-day manual
process involving scattered documentation, toolchain issues, and integration
pain. Ludus automates this end-to-end for any UE5 project.

## Target Pipeline

```
1. Validate prerequisites     (ludus init)
2. Build UE5 from source      (ludus engine build)
3. Build game Linux server     (ludus game build)
4. Containerize server         (ludus container build)
5. Push to Amazon ECR          (ludus container push)
6. Deploy to GameLift          (ludus deploy fleet)
7. Create test game session    (ludus deploy session)
```

Each step can be run independently or as a full pipeline via `ludus run`.

## Technology Choices

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Go | Strongly typed, single binary distribution, no runtime deps, good concurrency for orchestrating builds |
| CLI framework | Cobra | Industry standard (used by kubectl, docker, gh) |
| Config | Viper + YAML | Cobra's companion library, ludus.yaml for project config |
| AWS SDK | AWS SDK for Go v2 | Native Go SDK for GameLift, ECR, IAM |
| Container | Docker | Standard container runtime, GameLift requires Linux containers |
| IaC | CloudFormation | Atomic deployments with rollback and version control (`ludus deploy stack`) |

## Target Environment

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| OS | Ubuntu Linux (x86_64) | Ubuntu 22.04+ |
| CPU | 8 cores | 16+ cores |
| RAM | 16 GB | 32 GB |
| Disk | 350 GB free | 500 GB free |
| Go | 1.24+ | Latest stable |
| Docker | 20.10+ | Latest stable |
| AWS CLI | v2 | Latest stable |

RAM is critical — UE5 linking jobs can spike to 8+ GB each. The `--jobs` flag
on `ludus engine build` controls parallelism to prevent OOM kills.

## Unreal Engine Details

- **Source build required** — Epic launcher builds cannot produce dedicated server targets
- **Epic Games GitHub access required** — UE5 source is gated behind Epic account linking
- **Cross-compilation** from Linux uses clang-based toolchain (auto-downloaded by Setup.sh)
- **Game-agnostic** — any UE5 project with a `*Server.Target.cs` is supported; target names are configurable via `ludus.yaml`
- **Default server map**: configurable per project (Lyra default: `L_Expanse`)
- **Server port**: 7777 (UDP)

### Build Pipeline (what `ludus engine build` + `ludus game build` orchestrate)

```bash
# Engine setup
cd <engine_path>
./Setup.sh                    # Downloads ~40GB of dependencies
./GenerateProjectFiles.sh     # Generates makefiles

# Engine compilation (editor and tools only — game server is built via RunUAT)
make -j<N> ShaderCompileWorker  # Build tools first
make -j<N> UnrealEditor         # Editor target

# Game server packaging (via RunUAT)
./Engine/Build/BatchFiles/RunUAT.sh BuildCookRun \
  -project=<ProjectPath>/<Project>.uproject \
  -platform=Linux \
  -server -noclient \
  -servertargetname=<ServerTarget> \
  -cook -build -stage -package -archive \
  -archivedirectory=<output_dir>
```

## GameLift Integration

Ludus uses Amazon's official [Game Server Wrapper](https://github.com/amazon-gamelift/amazon-gamelift-servers-game-server-wrapper)
(v1.1.0) — a Go binary that runs as PID 1 in the container and handles all
GameLift SDK lifecycle calls (InitSDK, ProcessReady, health checks,
ProcessEnding). The UE5 server runs unmodified as a child process.

This zero-code-change approach means Ludus works with any UE5 project and any
engine version without patching game source.

### Container Structure

```dockerfile
FROM public.ecr.aws/amazonlinux/amazonlinux:2023
# Install runtime libraries (libicu, libnsl, libstdc++)
# Create non-root user (REQUIRED for Unreal servers)
# Copy server build + wrapper binary + wrapper config.yaml
# Wrapper is ENTRYPOINT — launches game server as child process
ENTRYPOINT ["./amazon-gamelift-servers-game-server-wrapper"]
```

The wrapper reads a `config.yaml` that specifies the game server executable
path, launch arguments, and port configuration. Ludus generates this config
automatically based on `ludus.yaml` settings.

### GameLift Deployment Flow

Current deployment uses imperative AWS SDK API calls (`ludus deploy fleet`):

1. Push Docker image to Amazon ECR (same region as fleet)
2. Create container group definition (references ECR image, port config, SDK version)
3. Wait for status COPYING → READY (GameLift snapshots the image)
4. Create IAM role with `GameLiftContainerFleetPolicy`
5. Create container fleet (instance type, container group def, inbound permissions)
6. Wait for fleet to become ACTIVE
7. Create game session for testing

A declarative CloudFormation-based alternative (`ludus deploy stack`) provides
atomic deployments with automatic rollback on failure.

## AWS Resources Referenced

### GitHub Repositories

| Repository | What It Provides |
|-----------|-----------------|
| [amazon-gamelift/amazon-gamelift-toolkit](https://github.com/amazon-gamelift/amazon-gamelift-toolkit) | Containers Starter Kit (Dockerfile, Go wrapper, CloudFormation) |
| [amazon-gamelift/amazon-gamelift-plugin-unreal](https://github.com/amazon-gamelift/amazon-gamelift-plugin-unreal) | GameLift Unreal Plugin v3.1.0 (Server SDK 5.4.0, supports UE 5.0-5.6) |
| [amazon-gamelift/amazon-gamelift-servers-game-server-wrapper](https://github.com/amazon-gamelift/amazon-gamelift-servers-game-server-wrapper) | Go wrapper for quick onboarding without direct SDK integration |
| [amazon-gamelift/amazon-gamelift-servers-go-server-sdk](https://github.com/amazon-gamelift/amazon-gamelift-servers-go-server-sdk) | Go Server SDK (used by the wrapper) |
| [aws-samples/amazon-gamelift-unreal-engine](https://github.com/aws-samples/amazon-gamelift-unreal-engine) | Video series sample code (Lambda, Cognito, UE client) |
| [aws-games/cloud-game-development-toolkit](https://github.com/aws-games/cloud-game-development-toolkit) | CGD Toolkit — Terraform modules for Unreal Horde, Perforce, Jenkins, etc. (future integration candidate) |

### Key Documentation

| Topic | URL |
|-------|-----|
| GameLift Containers Roadmap | https://docs.aws.amazon.com/gamelift/latest/developerguide/gamelift-roadmap-containers.html |
| Container Group Definitions | https://docs.aws.amazon.com/gamelift/latest/developerguide/containers-create-groups.html |
| Unreal Plugin Docs | https://docs.aws.amazon.com/gamelift/latest/developerguide/unreal-plugin.html |
| Container Fleet Deployment | https://docs.aws.amazon.com/gamelift/latest/developerguide/unreal-plugin-container.html |
| UE5 Dedicated Server Setup | https://dev.epicgames.com/documentation/en-us/unreal-engine/setting-up-dedicated-servers-in-unreal-engine |
| UE5 Linux Cross-Compile | https://dev.epicgames.com/documentation/en-us/unreal-engine/cross-compiling-for-linux |

### Community Resources

| Resource | URL |
|----------|-----|
| Unreal Containers Hub | https://unrealcontainers.com/ |
| ue4-docker (supports UE5) | https://github.com/adamrehn/ue4-docker |
| UE5 Dedicated Server Dockerfile | https://github.com/LeMustelide/ue5-dedicated-server-docker |

## Project Structure

```
ludus/
├── main.go                          # Entry point
├── go.mod / go.sum                  # Go module
├── ludus.example.yaml               # Config template
├── SPEC.md                          # This file
├── CLAUDE.md                        # Claude Code guidance
├── cmd/
│   ├── root/root.go                 # Root command, subcommand registration
│   ├── globals/globals.go           # Global mutable state (Cfg, Verbose, DryRun)
│   ├── init/init.go                 # ludus init
│   ├── engine/engine.go             # ludus engine build|setup
│   ├── game/game.go                 # ludus game build|client
│   ├── container/container.go       # ludus container build|push
│   ├── deploy/deploy.go             # ludus deploy fleet|stack|session|destroy
│   ├── connect/connect.go           # ludus connect
│   ├── status/status.go             # ludus status
│   ├── pipeline/pipeline.go         # ludus run (full pipeline)
│   ├── mcp/                         # ludus mcp (MCP server, multiple files)
│   └── ci/                          # ludus ci init|runner (CI integration)
└── internal/
    ├── config/config.go             # Config structs + defaults
    ├── prereq/                      # Prerequisite validation (platform-tagged files)
    ├── toolchain/                   # Engine version detection, cross-compile toolchain
    ├── engine/builder.go            # UE5 engine build orchestration
    ├── game/builder.go              # Game server/client build orchestration
    ├── container/builder.go         # Docker image creation + GameLift wrapper
    ├── deploy/target.go             # Deploy target interface
    ├── gamelift/deployer.go         # GameLift deployment (AWS SDK)
    ├── stack/                       # CloudFormation stack deployment
    ├── tags/tags.go                 # Centralized AWS resource tagging
    ├── binary/exporter.go           # Binary export deployment target
    ├── ci/                          # CI workflow generation + runner management
    ├── state/state.go               # Persistent state (.ludus/state.json)
    ├── status/status.go             # Stage status checks
    └── runner/runner.go             # Shell command executor
```

## Implemented

- [x] Full CLI command hierarchy with all pipeline stages
- [x] Prerequisite validation with auto-fix on Windows (`ludus init --fix`)
- [x] Engine build orchestration (Linux + Windows)
- [x] Game server + client build via RunUAT (game-agnostic, config-driven targets)
- [x] Container build with GameLift Game Server Wrapper (zero game code changes)
- [x] ECR push
- [x] GameLift fleet deployment via imperative AWS SDK calls
- [x] Game session creation and client connect
- [x] Pluggable deployment targets (`gamelift`, `stack`, `binary`)
- [x] Cross-compile toolchain management (engine version → clang SDK mapping)
- [x] MCP server for AI agent orchestration (13 tools)
- [x] GitHub Actions CI integration (workflow generation + self-hosted runner management)
- [x] Cross-platform support (Linux server pipeline, Windows client build + connect)
- [x] Persistent state tracking (`.ludus/state.json`)
- [x] CloudFormation-based deployment (`ludus deploy stack`) with atomic rollback
- [x] Centralized, configurable AWS resource tagging (`aws.tags` in `ludus.yaml`)

## Future / Out of Scope (for now)

- **Docker build backend** — Build via a private engine Docker image as alternative to native builds
- **Build caching** — Skip unchanged pipeline stages based on file hashes
- **BuildGraph / DAG-based orchestration** — Parallel builds, distributed execution, artifact caching
- **CGD Toolkit integration** — Use Ludus within the Cloud Game Development Toolkit's CI/CD pipeline
- **FlexMatch matchmaking** — GameLift matchmaking configuration
- **Multi-region deployment** — Fleet replication across regions
- **Client-side auth stack** — Cognito + API Gateway for player authentication
- **WSL2 support** — OS prereq check update, .wslconfig memory guidance
- **macOS support** — Mac-specific engine scripts, cross-compilation strategy
