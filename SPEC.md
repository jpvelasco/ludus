# Ludus — Project Specification

> Latin for "game" / gladiator training school. A CLI tool that streamlines the
> end-to-end pipeline for deploying Unreal Engine 5 Lyra dedicated servers to
> AWS GameLift Containers.

## Problem Statement

Getting an Unreal Engine 5 dedicated server built from source, compiled as a
Lyra sample project, containerized for Linux, and deployed to AWS GameLift
Containers is a multi-day manual process involving scattered documentation,
toolchain issues, and integration pain. Ludus automates this end-to-end.

## Target Pipeline

```
1. Validate prerequisites     (ludus init)
2. Build UE5 from source      (ludus engine build)
3. Integrate GameLift SDK      (ludus lyra integrate-gamelift)
4. Build Lyra Linux server     (ludus lyra build)
5. Containerize server         (ludus container build)
6. Push to Amazon ECR          (ludus container push)
7. Deploy to GameLift          (ludus deploy fleet)
8. Create test game session    (ludus deploy session)
```

Each step can be run independently or as a full pipeline via `ludus run`.

## Technology Choices

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Go | Strongly typed, single binary distribution, no runtime deps, good concurrency for orchestrating builds |
| CLI framework | Cobra | Industry standard (used by kubectl, docker, gh) |
| Config | Viper + YAML | Cobra's companion library, ludus.yaml for project config |
| AWS SDK | AWS SDK for Go v2 | Native Go SDK for GameLift, ECR, CloudFormation, IAM |
| Container | Docker | Standard container runtime, GameLift requires Linux containers |
| IaC | CloudFormation | Matches GameLift Containers Starter Kit patterns |

## Target Environment

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| OS | Ubuntu Linux (x86_64) | Ubuntu 22.04+ |
| CPU | 8 cores | 16+ cores |
| RAM | 16 GB | 32 GB |
| Disk | 350 GB free | 500 GB free |
| Go | 1.21+ | Latest stable |
| Docker | 20.10+ | Latest stable |
| AWS CLI | v2 | Latest stable |

RAM is critical — UE5 linking jobs can spike to 8+ GB each. The `--jobs` flag
on `ludus engine build` controls parallelism to prevent OOM kills.

## Unreal Engine Details

- **Source build required** — Epic launcher builds cannot produce dedicated server targets
- **Epic Games GitHub access required** — UE5 source is gated behind Epic account linking
- **Cross-compilation** from Linux uses clang-based toolchain (auto-downloaded by Setup.sh)
- **Lyra already ships with** `LyraServer.Target.cs` and `LyraClient.Target.cs`
- **Default server map**: `L_Expanse` (bypasses main menu which doesn't work on dedicated servers)
- **Server port**: 7777 (UDP)

### Build Pipeline (what `ludus engine build` + `ludus lyra build` will orchestrate)

```bash
# Engine setup
cd <engine_path>
./Setup.sh                    # Downloads ~40GB of dependencies
./GenerateProjectFiles.sh     # Generates makefiles

# Engine compilation
make -j<N> ShaderCompileWorker  # Build tools first
make -j<N> UnrealEditor         # Editor target
make -j<N> LyraServer           # Server target (linux)

# Lyra server packaging (via RunUAT)
./Engine/Build/BatchFiles/RunUAT.sh BuildCookRun \
  -project=<LyraPath>/Lyra.uproject \
  -platform=Linux \
  -server -noclient \
  -cook -build -stage -package -archive \
  -archivedirectory=<output_dir>
```

## GameLift Integration

### Two SDK Integration Approaches

1. **Go SDK Wrapper (no Lyra code changes)** — From the GameLift Containers Starter Kit.
   A Go binary runs alongside the server, handles all SDK lifecycle calls, and
   exposes game session data via HTTP on localhost:8090. The Lyra server binary
   runs unmodified.

2. **Direct C++ SDK Integration** — Add GameLiftServerSDK module to
   `LyraGame.Build.cs`, create a GameLift-aware GameMode subclass that calls
   InitSDK/ProcessReady/ActivateGameSession. More control, but requires patching
   Lyra source.

**Decision**: Support both. Default to the Go wrapper for quick setup; offer
`ludus lyra integrate-gamelift` for teams that want direct integration.

### Container Structure (based on GameLift Containers Starter Kit)

```dockerfile
FROM public.ecr.aws/amazonlinux/amazonlinux:latest
# Install Go for SDK wrapper
# Create non-root user (REQUIRED for Unreal servers)
# Copy server build + Go wrapper + wrapper.sh
# Entrypoint: wrapper.sh
```

The wrapper.sh starts the Go SDK wrapper in background, then launches the Lyra
server binary. On server exit, it signals the wrapper to call ProcessEnding().

### GameLift Deployment Flow

1. Push Docker image to Amazon ECR (same region as fleet)
2. Create container group definition (references ECR image, port config, SDK version)
3. Wait for status COPYING → READY (GameLift snapshots the image)
4. Create IAM role with `GameLiftContainerFleetPolicy`
5. Create container fleet (instance type, container group def, inbound permissions)
6. Wait for fleet to become ACTIVE
7. Create game session for testing

## AWS Resources Referenced

### GitHub Repositories

| Repository | What It Provides |
|-----------|-----------------|
| [amazon-gamelift/amazon-gamelift-toolkit](https://github.com/amazon-gamelift/amazon-gamelift-toolkit) | Containers Starter Kit (Dockerfile, Go wrapper, CloudFormation, wrapper.sh) |
| [amazon-gamelift/amazon-gamelift-plugin-unreal](https://github.com/amazon-gamelift/amazon-gamelift-plugin-unreal) | GameLift Unreal Plugin v3.1.0 (Server SDK 5.4.0, supports UE 5.0-5.6) |
| [amazon-gamelift/amazon-gamelift-servers-game-server-wrapper](https://github.com/amazon-gamelift/amazon-gamelift-servers-game-server-wrapper) | Go wrapper for quick onboarding without direct SDK integration |
| [amazon-gamelift/amazon-gamelift-servers-cpp-server-sdk](https://github.com/amazon-gamelift/amazon-gamelift-servers-cpp-server-sdk) | Standalone C++ Server SDK 5.4.0 |
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
| UE5 Dedicated Server Setup (uses Lyra) | https://dev.epicgames.com/documentation/en-us/unreal-engine/setting-up-dedicated-servers-in-unreal-engine |
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
├── .gitignore
├── cmd/
│   ├── root/
│   │   ├── root.go                  # Root command, subcommand registration
│   │   └── init.go                  # ludus init
│   ├── engine/engine.go             # ludus engine build|setup
│   ├── lyra/lyra.go                 # ludus lyra build|integrate-gamelift
│   ├── container/container.go       # ludus container build|push
│   ├── deploy/deploy.go             # ludus deploy fleet|stack|session
│   ├── status/status.go             # ludus status
│   └── pipeline/pipeline.go         # ludus run
└── internal/
    ├── config/config.go             # Config structs + defaults
    ├── prereq/checker.go            # Prerequisite validation
    ├── engine/builder.go            # UE5 build orchestration
    ├── lyra/builder.go              # Lyra server build orchestration
    ├── container/builder.go         # Docker image creation
    ├── gamelift/deployer.go         # GameLift deployment
    └── runner/runner.go             # Shell command executor
```

## Current State

- [x] Project scaffolded with full CLI command hierarchy
- [x] All commands stubbed with descriptive output
- [x] Internal packages with interfaces and types defined
- [x] Config structs with sensible defaults
- [x] Prerequisite checker skeleton
- [x] Command runner utility
- [x] Project compiles cleanly
- [x] Git repo initialized with initial commit

## Next Steps (Implementation Order)

1. **`ludus init`** — Wire up prereq checker (disk, RAM, Docker, AWS CLI, engine path)
2. **`ludus engine setup`** — Run Setup.sh with output streaming
3. **`ludus engine build`** — Generate project files, compile engine with job limiting
4. **`ludus lyra build`** — RunUAT BuildCookRun for Linux server
5. **`ludus lyra integrate-gamelift`** — Patch Lyra with GameLift SDK
6. **`ludus container build`** — Generate Dockerfile, build image
7. **`ludus container push`** — ECR auth + push
8. **`ludus deploy fleet`** — Container group def + fleet creation via AWS SDK
9. **`ludus deploy session`** — Test game session creation
10. **`ludus status`** — Wire up stage checks
11. **`ludus run`** — Connect all stages into pipeline

## Future / Out of Scope (for now)

- **CGD Toolkit integration** — Potentially use Ludus as a building block within
  the Cloud Game Development Toolkit's Unreal Horde CI/CD pipeline
- **FlexMatch matchmaking** — GameLift matchmaking configuration
- **Multi-region deployment** — Fleet replication across regions
- **Client-side auth stack** — Cognito + API Gateway for player authentication
- **Automated testing** — Integration tests against GameLift Anywhere
- **Windows support** — Currently Linux-only (matches GameLift Containers requirement)

## Dev Environment

As of project creation:
- **Machine**: AMD Ryzen 7 2700X (8 cores), 7.7 GB RAM, 216 GB disk (169 GB free)
- **OS**: Ubuntu, Linux 6.17.0-14-generic
- **Go**: 1.23.5
- **UE Source**: UnrealEngine-5.7.3-release (3.1 GB, cloned but not built)
- **Pending upgrades**: RAM to 32 GB, disk to 500 GB+
