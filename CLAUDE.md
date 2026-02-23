# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ludus is a Go CLI tool that automates the end-to-end pipeline for deploying Unreal Engine 5 dedicated servers to AWS GameLift Containers. It orchestrates: UE5 source builds → game server compilation → Docker containerization → ECR push → GameLift fleet deployment. While Lyra (Epic's sample game) is the default project, Ludus supports any UE5 game with dedicated server targets.

## Build & Run Commands

```bash
go build -o ludus -v         # Build (Linux/macOS)
go build -o ludus.exe -v .   # Build (Windows)
GOOS=windows go build -o /dev/null .  # Cross-compile check (from Linux)
go mod tidy                  # Clean up module dependencies
go vet ./...                 # Static analysis
golangci-lint run ./...      # Lint (must pass before commit)
go test ./...                # Run all tests
go test -v ./internal/toolchain  # Run tests for a single package
```

Run the CLI after building:
```bash
./ludus --help
./ludus init --verbose       # Validate prerequisites
./ludus init --fix           # Auto-fix issues where possible (Windows: VS components, BuildConfiguration.xml, NNERuntimeORT patch)
./ludus run --dry-run        # Full pipeline dry run
./ludus run --verbose --skip-engine  # Skip engine build stage
```

## Architecture

### Entry point

`main.go` → `root.Execute()` → Cobra command dispatch. The root command's `PersistentPreRunE` loads config via `config.Load()` into `globals.Cfg` before any subcommand runs. `SilenceUsage: true` is set on `rootCmd` so Cobra only prints the error message on failure, not the full usage text.

### Command layer (`cmd/`)

Each subcommand lives in its own package under `cmd/` and exports a `Cmd *cobra.Command` variable. All commands are registered in `cmd/root/root.go` via `rootCmd.AddCommand()`.

Command hierarchy:
```
ludus init                          --fix (auto-remediate on Windows)
ludus engine [build|setup]         --jobs/-j (0=auto)
ludus game [build|client|integrate-gamelift]  --skip-cook, --platform (Linux|Win64)
ludus container [build|push]       --tag/-t, --no-cache
ludus deploy [fleet|stack|session|destroy]  --target, --region, --instance-type, --fleet-name
ludus connect                      --address (ip:port override)
ludus status                       # checks: engine source/build, game build, client build, container image, fleet, session
ludus run                          # full pipeline (6+ stages)
  --skip-engine, --skip-game, --skip-container, --skip-deploy, --with-client
```

Global persistent flags (`cmd/root/root.go`): `--config`, `--verbose/-v`, `--json`, `--dry-run`.

Global mutable state lives in `cmd/globals/globals.go`: `Cfg`, `Verbose`, `JSONOutput`, `DryRun`.

### Implementation layer (`internal/`)

All business logic is in `internal/` (unexported to consumers):

- **`config`** — `Config` struct with typed sub-structs (`EngineConfig`, `GameConfig`, `ContainerConfig`, `DeployConfig`, `GameLiftConfig`, `AWSConfig`). `GameConfig` includes `ProjectName`, `ServerTarget`, `ClientTarget`, `GameTarget` fields with resolver methods (`ResolvedServerTarget()`, etc.) that default to `ProjectName+"Server"` etc. `Defaults()` returns sensible defaults with `ProjectName: "Lyra"`. `Load()` reads `ludus.yaml` via Viper, expands relative paths, gracefully returns defaults if file is missing. Backward compat: if `lyra:` key present but no `game:` key, migrates and prints deprecation warning to stderr.
- **`runner`** — Shell command executor. `Run()` and `RunInDir()` use `exec.CommandContext`. Supports `Verbose` (prints `+ command` before running) and `DryRun` (prints without executing) modes. Streams stdout/stderr.
- **`prereq`** — `Checker` with `RunAll()` returning `[]CheckResult`. Cross-platform checks: OS (linux/windows), engine source, toolchain (via `toolchain` package), game content (Lyra-specific or generic via `ContentValidationConfig`), Docker (warn-only on Windows), AWS CLI, Git, Go, disk space (100 GB), RAM (16 GB). Windows-specific checks via `platformChecks()`: Visual Studio workloads/components (via vswhere), MSVC 14.38 toolchain config (`BuildConfiguration.xml`), Windows SDK version, NNERuntimeORT INITGUID patch. `CheckResult` has `Warning bool` for non-fatal issues. `Checker.Fix bool` gates auto-remediation (`--fix` flag). Disk, memory, and platform checks use build-tagged files.
- **`toolchain`** — Engine version detection and cross-compile toolchain validation. `ParseBuildVersion()` reads `Engine/Build/Build.version` JSON. `DetectEngineVersion()` tries Build.version first, falls back to config string. `LookupToolchain()` maps engine major.minor (5.4→clang-16, 5.5/5.6→clang-18, 5.7→clang-20) to `ToolchainSpec`. `CheckToolchain()` orchestrates detection + platform-specific search: Linux scans `Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/Linux_x64/` and `LINUX_MULTIARCH_ROOT`; Windows checks `LINUX_MULTIARCH_ROOT` only. No build tags — uses `runtime.GOOS` for platform branching.
- **`engine`** — `Builder` for UE5 compilation. Linux: Setup.sh, GenerateProjectFiles.sh, make. Windows: Setup.bat, GenerateProjectFiles.bat, Build.bat. Targets: `ShaderCompileWorker` and `UnrealEditor` only (game server is built via RunUAT in the game stage). Auto-detects max jobs from RAM (8 GB per job).
- **`game`** — `Builder` for UE5 game packaging via RunUAT BuildCookRun. Cross-platform: `resolveRunUAT()` selects `cmd /c RunUAT.bat` (Windows) or `bash RunUAT.sh` (Linux). Uses relative script paths to avoid spaces-in-path issues with `cmd /c`. Path arguments are quoted (`-project="..."`) for the same reason. Pre-build fixups: writes `Directory.Build.props` (`NuGetAuditLevel=critical`) and ensures `DefaultServerTarget` in DefaultEngine.ini (skips gracefully if INI structure doesn't match expected format). `BuildClient()` supports `--platform` flag (Linux, Win64). All target names (`-servertargetname`, binary paths) are config-driven via `BuildOptions`.
- **`container`** — `Builder` for Dockerfile generation (Amazon Linux 2023, non-root user), `docker build`, and ECR push (login + tag + push). Project name and server target are parameterized in generated Dockerfile and wrapper config.
- **`deploy`** — `Target` interface abstracting deployment backends, with `Capabilities` (what the target needs/supports), `Deploy()`, `Status()`, `Destroy()` methods. Optional `SessionManager` interface for targets that support game sessions. Shared types: `DeployInput`, `DeployResult`, `DeployStatus`, `SessionInfo`. Implementations are in `gamelift` and `binary` packages; target resolution lives in `cmd/globals/resolve.go`.
- **`gamelift`** — `Deployer` for AWS GameLift via SDK v2. Creates container group definitions, IAM roles, fleets. Polls with 15s intervals / 30min timeout. Tags resources with `ludus:managed` and `ludus:fleet-name`. `Destroy()` tears down in reverse order, tolerating not-found errors. `TargetAdapter` wraps `Deployer` to implement `deploy.Target` and `deploy.SessionManager`. `CreateGameSession` returns `*GameSessionInfo` (SessionID, IPAddress, Port). `DescribeGameSession` checks session liveness.
- **`binary`** — `Exporter` implements `deploy.Target` for simple file export. `Deploy()` copies the server build directory to a configurable output dir via `cp -a`. `Status()` checks if the output dir exists and has files. `Destroy()` removes the output dir.
- **`state`** — Persistent state in `.ludus/state.json`. Tracks fleet (ID, status), session (ID, IP, port), client build (binary path, platform, output dir), and deploy (target name, status, detail). Read-modify-write via `Load()`/`Save()` with typed update helpers (`UpdateFleet`, `UpdateSession`, `UpdateClient`, `UpdateDeploy`, `ClearSession`, `ClearFleet`).

### Platform-specific code

Build-tagged files use `//go:build` tags for platform-specific implementations:

- `internal/prereq/checker_windows.go` / `checker_unix.go` — Disk space (Windows: `GetDiskFreeSpaceExW`, Unix: `syscall.Statfs`), memory checks (Windows: `GlobalMemoryStatusEx`, Unix: `/proc/meminfo`), and `platformChecks()` dispatch (Windows: VS/MSVC/SDK/patch checks; Unix: no-op)
- `cmd/connect/launch_windows.go` / `launch_unix.go` — Client launch (Windows: `os/exec.Command` to start as child process, Unix: `syscall.Exec` to replace current process)
- `cmd/status/status.go` — Uses `runtime.GOOS` to check for `Setup.bat`/`UnrealEditor.exe` (Windows) vs `Setup.sh`/`UnrealEditor` (Linux)

### Patterns

- **Builder pattern**: Each major operation has a `Builder`/`Deployer` type with `New*(opts)` constructor, operation methods, and structured result types (`BuildResult`, `FleetStatus`).
- **Context threading**: All builders/deployers accept `context.Context` for cancellation and timeouts.
- **Runner abstraction**: Commands never call `exec.Command` directly — they use `runner.Runner` which handles verbose/dry-run modes uniformly.
- **Pluggable targets**: Deployment is abstracted behind `deploy.Target` interface. `cmd/globals.ResolveTarget()` is the factory that creates the appropriate target based on config (`deploy.target` in `ludus.yaml`) or CLI flag (`--target`). The pipeline checks `target.Capabilities()` to skip container/push stages for targets that don't need them. GameLift-specific commands (`fleet`, `session`) still use the direct `gamelift.Deployer` when needed; generic commands (`destroy`, pipeline deploy) use the interface.
- **Config override**: Deploy subcommands accept `--target`, `--region`, `--instance-type`, `--fleet-name` flags that override `ludus.yaml` values.
- **State persistence**: Deploy and client-build commands write to `.ludus/state.json` so downstream commands (`connect`, `status`) can resolve fleet/session/client info without re-querying AWS.
- **Config-driven targets**: Game project name, server target, client target, and game target are all configurable via `ludus.yaml`. Defaults derive from `ProjectName` (e.g., `ProjectName+"Server"` for `ServerTarget`). Lyra-specific behavior (auto-detection of project path, content validation with plugin dirs) is preserved as a fallback when `ProjectName == "Lyra"`.

## Configuration

Config template: `ludus.example.yaml`. User config: `ludus.yaml` (gitignored). Key settings: engine source path, max compile jobs (0 = auto-detect from RAM), project name (`Lyra` default), server map (`L_Expanse`), server port (7777 UDP), deploy target (`gamelift` default, or `binary`), GameLift instance type (`c6i.large`), container group name, AWS region/account.

The `game:` section supports any UE5 project:
```yaml
game:
  projectName: "MyGame"           # Required for non-Lyra projects
  projectPath: "/path/to/MyGame.uproject"  # Required for non-Lyra projects
  serverTarget: "MyGameServer"    # Optional, defaults to <projectName>Server
  clientTarget: "MyGame"          # Optional, defaults to <projectName>Game
  gameTarget: "MyGame"            # Optional, defaults to <projectName>Game
  serverMap: "MyDefaultMap"
  contentValidation:
    disabled: false               # Set true to skip content checks
    contentMarkerFile: "Content/SomeAsset.uasset"  # Optional marker to verify
```

Backward compatibility: if `ludus.yaml` uses the old `lyra:` key, values are migrated to `game:` automatically with a deprecation warning.

## Key Domain Context

- UE5 must be built from source (Epic launcher builds can't produce dedicated server targets)
- Lyra Content assets are NOT in the GitHub source repo — must be downloaded from the Epic Games Launcher Marketplace ("Lyra Starter Game") and the **entire project** must be overlaid onto the engine's `Samples/Games/Lyra/` directory. This includes both the top-level `Content/` directory AND `Plugins/GameFeatures/*/Content/` directories (ShooterCore, ShooterExplorer, ShooterMaps, ShooterTests, TopDownArena each have their own Content folder with GameFeatureData assets). Missing plugin content causes cook failures (ExitCode=25, "GameFeatureData is missing"). The Epic Games Launcher does not run on Linux; Windows or macOS required for this one-time download.
- RAM is critical — UE5 linking can spike 8+ GB per job; `maxJobs` controls parallelism to prevent OOM
- UE 5.6 Lyra has multiple server targets (LyraServer, LyraServerEOS, LyraServerSteam, LyraServerSteamEOS) — `DefaultServerTarget=LyraServer` must be set in DefaultEngine.ini
- UE 5.6's Gauntlet test framework directly depends on Magick.NET 14.7.0 with known CVEs; combined with TreatWarningsAsErrors, AutomationTool script modules fail to compile without `NuGetAuditLevel=critical` in a Directory.Build.props at `Engine/Source/Programs/`
- GameLift integration has two approaches: Go SDK wrapper (no game code changes, default) and direct C++ SDK integration (`ludus game integrate-gamelift`)
- Container must run as non-root user (Unreal server requirement)
- Server builds are Linux x86_64 only (matches GameLift Containers requirement)
- Client builds support Linux and Win64; native Win64 builds work if UE5 is built from source on Windows
- `ludus connect` launches the client directly on both platforms (Windows: `os/exec` child process, Linux: `syscall.Exec` process replacement). On Linux with a Win64 client, it prints copy/run instructions instead.
- UE5 content cooking requires 24+ GB RAM; 32 GB recommended. On Ubuntu, `systemd-oomd` kills the cook process at 50% memory pressure — disable it before building (`sudo systemctl disable --now systemd-oomd systemd-oomd.socket`)
- UE 5.6.1 on Windows requires specific source patches and toolchain versions — see `UE_SOURCE_PATCHES.md` for details (INITGUID fix for NNERuntimeORT on SDK >= 26100, MSVC 14.38 toolchain requirement)

## CI / Linting

GitHub Actions CI (`.github/workflows/ci.yml`) runs on push/PR to `main`:

- **Lint** — `golangci-lint` on both Ubuntu and Windows (separate jobs to cover platform-specific build tags)
- **Build** — `go build` + `go vet` on both OSes
- **Test** — `go test` on both OSes

Lint config (`.golangci.yml`) enables: errcheck, govet, ineffassign, staticcheck, unused, gosimple, gocritic, misspell, unconvert, gosec. Gosec exclusions: G115 (integer overflow — port numbers and memory math are bounded), G204 (subprocess with variable — intentional in runner package), G306 (WriteFile 0644 — build config files need to be readable).

Run lint locally:
```bash
golangci-lint run ./...
```

Pre-commit hooks (`.hooks/pre-commit`) run `go build`, `golangci-lint` (falls back to `go vet` if not installed), and `go test` before each commit. Activate with:
```bash
git config core.hooksPath .hooks
```

## Dependencies

Go 1.23.5, Cobra v1.10.2 (CLI), Viper v1.21.0 (config/YAML), AWS SDK for Go v2 (GameLift, IAM, config, credentials, STS/SSO for auth).

## Cross-Platform Notes

The server pipeline (engine build → container → deploy) is Linux-only. The client build and connect commands work on both Linux and Windows.

On Windows:
1. `go build -o ludus.exe -v .`
2. Configure `ludus.yaml` with `engine.sourcePath` pointing to the Windows UE5 source
3. `ludus.exe game client --platform Win64 --verbose` — builds the Win64 game client
4. `ludus.exe deploy session` — creates a game session (or copy `.ludus/state.json` from the Linux machine)
5. `ludus.exe connect` — launches the client directly and connects to the server

Windows-specific prerequisites detected by `ludus init` (auto-fixed with `--fix` where noted):
- Visual Studio with "Desktop development with C++", "Game development with C++", and MSVC v14.38 component **(auto-fix: launches VS Installer in passive mode)**
- `BuildConfiguration.xml` at `%APPDATA%\Unreal Engine\UnrealBuildTool\` to pin MSVC 14.38.33130 **(auto-fix)**
- Windows SDK version detection; warns if build >= 26100 (requires NNERuntimeORT patch)
- NNERuntimeORT INITGUID patch in `Engine/Plugins/NNE/NNERuntimeORT/Source/NNERuntimeORT/NNERuntimeORT.Build.cs` **(auto-fix)**

## Not Yet Implemented

- `ludus game integrate-gamelift` — C++ GameLift SDK patching into project source (command exists as stub)
- `ludus deploy stack` — CloudFormation-based deployment

## Validated End-to-End

- Linux: Engine → Lyra server → container → ECR → GameLift fleet → game sessions (UDP connectivity confirmed)
- Windows: Win64 client built → connected to GameLift fleet → played on live Linux server container

## Competitive Landscape: UET (Redpoint Games)

Redpoint UET (~130 stars) is the closest existing tool to Ludus. Understanding where it overlaps and diverges informs roadmap priorities.

**Architecture**: UET is a developer-friendly orchestration layer on top of Epic's BuildGraph (XML DAG engine) and UAT. It dynamically generates BuildGraph XML so users never write it by hand. Ludus bypasses BuildGraph entirely and calls UAT/build commands directly — simpler, fewer moving parts, but less flexible for complex multi-target orchestration.

**What UET does that Ludus doesn't (yet)**:
- Automatic BuildGraph generation — one-command workflows that produce optimized DAGs for multi-target builds (editor + client + server + tools in one pipeline). Ludus's linear pipeline handles the common case; the long-term DAG roadmap item is where the two converge.
- UEFS (engine virtualization) — network-mounted portable engine images, multiple engine versions per machine without reinstalling. Partially addressed by the Docker build backend roadmap item, but UEFS is more granular.
- Distributed builds and tests — parallel execution across machines with memory-aware pooling. Ludus is single-machine today.
- Plugin packaging — engine-version-aware builds with Marketplace-ready output. Niche, not on Ludus's roadmap.
- Store deployment providers — Steam, Google Play, Meta/Quest, BackblazeB2, Docker/Helm, custom scripts. Ludus doesn't target storefront distribution (it's focused on dedicated server deployment).
- SDK/environment auto-download — UET downloads and configures required SDKs automatically. Ludus detects and validates (toolchain management) but does not auto-download, since Epic's toolchain URLs change between versions.
- CI config generation — UET generates GitLab CI configs. Ludus has this on the mid-term roadmap (`ludus ci init`) targeting GitHub Actions, GitLab CI, and shell scripts.

**What Ludus does that UET doesn't**:
- **Server infrastructure deployment** — UET has deployment providers for distribution to stores (Steam, Google Play, Meta/Quest, BackblazeB2) and Docker/Helm registries, but no server infrastructure orchestration. Ludus covers the full server lifecycle: GameLift fleet creation, container group definitions, game session management, and pluggable deployment targets (binary export, future Agones/Hathora). UET gets your build to a storefront; Ludus gets your dedicated server running in the cloud.
- **Dedicated server container pipeline** — Ludus generates server-specific Dockerfiles (non-root user, game server wrapper), pushes to ECR, and wires up GameLift container groups. UET's Docker provider is a generic image push, not server-aware.
- **AI agent orchestration** — Ludus's `--json`/`--dry-run`/idempotent commands are designed as an MCP execution layer. UET's BuildGraph DAGs are static with no runtime reasoning layer.
- **Cross-platform client workflow** — Build server on Linux, build client on Windows, connect to live fleet. UET doesn't address the player-side workflow.
- **Go single-binary distribution** — Ludus is one binary with no runtime dependencies. UET is a .NET tool requiring the .NET SDK.

**Where both overlap** (Ludus already covers):
- One-command workflows (`ludus run` vs `uet build`)
- SDK/toolchain auto-detection per engine version
- CI-friendly design (structured output, non-interactive modes)

**Strategic takeaway**: Ludus's differentiation is the **build-to-server-deployment pipeline** and **AI-native design**. UET's strengths are **build orchestration depth** via BuildGraph and **store distribution** (Steam, Google Play, Meta). The two tools are complementary more than competitive — UET gets builds to storefronts, Ludus gets dedicated servers running in the cloud. Competing on BuildGraph complexity is low-ROI — instead, Ludus should stay simple for the common case (single-target server builds) and invest in server deployment, MCP, and CI integration where UET has gaps.

## Roadmap

### Near-term (pipeline completeness)

- **~~Pluggable deployment targets~~** (done) — `deploy.Target` interface with `gamelift` and `binary` implementations. Pipeline stages gated by `target.Capabilities()`. `--target` flag on `deploy` subcommands. Future targets (Agones, Hathora) implement the same interface. See `internal/deploy/target.go`, `internal/gamelift/adapter.go`, `internal/binary/exporter.go`.
- **~~Cross-compile toolchain management~~** (done) — `toolchain` package auto-detects engine version from `Build.version` JSON (falls back to `engine.version` config), maps major.minor to required clang SDK (5.4→clang-16, 5.5/5.6→clang-18, 5.7→clang-20), and validates the toolchain directory exists. Linux: scans engine SDK dir and `LINUX_MULTIARCH_ROOT`. Windows: checks `LINUX_MULTIARCH_ROOT` (warn-only since server builds are Linux-only). Integrated into `ludus init` and pipeline prereq checks. See `internal/toolchain/toolchain.go`.
- **Eliminate engine source modifications** — Ludus currently patches the engine source in two places: (1) `Directory.Build.props` at `Engine/Source/Programs/` to suppress NuGet audit errors from Magick.NET CVEs in Gauntlet, and (2) NNERuntimeORT INITGUID patch in `Engine/Plugins/NNE/NNERuntimeORT/` on Windows with SDK >= 26100. These are version-specific workarounds for UE 5.6.1. To support arbitrary engine versions cleanly, find alternatives that don't touch engine source — e.g., MSBuild property overrides via environment variables, RunUAT flags, or conditional application only when the specific version/issue is detected. Goal: Ludus should work with any UE5 version without modifying a single engine file.

### Mid-term (CI/CD and broader adoption)

- **AI agent orchestration (MCP)** — Ludus's CLI architecture (discrete idempotent commands, `--json` output, `--dry-run` mode) makes it a natural execution layer for AI agents. The agent handles non-deterministic decisions (diagnosis, recovery, optimization), while ludus handles deterministic execution with predictable side effects. Key enablers: (1) ensure `--json` output covers all error paths with structured error objects (`stage`, `exit_code`, `hint`), (2) make `ludus status --json` comprehensive enough for an agent to decide the next action, (3) ship a separate `ludus-mcp` wrapper (or MCP config file) that exposes ludus commands as tools — this is glue code, not a core feature. The CLI boundary between agent and tool is the safety guarantee. UET has no equivalent — its BuildGraph DAGs are static with no runtime reasoning layer.
- **GitHub Actions / CI integration** — Generate CI workflow files (`ludus ci init`) for GitHub Actions, GitLab CI, or generic shell scripts. There is no game-ci equivalent for Unreal Engine (game-ci is Unity-only, 1.1k stars). Epic's EULA blocks distributing pre-built engine images, so CI requires self-hosted runners with a pre-built engine — Ludus can generate the workflow that assumes this setup. UET currently generates GitLab CI configs only.
- **Docker build backend** — Support building via a ue4-docker image (`ludus build --backend docker`) as an alternative to native engine builds. The Docker image contains a pre-compiled engine, eliminating local prereq complexity. Studios build the image once and reuse it across developers and CI. Lower priority than CI integration because ~85-90% of devs build natively today. UET's UEFS (network-mounted engine images) solves a similar problem at a different layer — more granular but heavier infrastructure.
- **Build caching** — Skip unchanged pipeline stages based on file hashes. Full engine+game rebuilds take hours; most runs only change game code. Track build artifacts and skip engine/cook stages when inputs haven't changed.

### Long-term (orchestration and ecosystem)

- **BuildGraph / DAG-based orchestration** — Define build steps as a directed acyclic graph instead of a linear pipeline. Enables parallelization (e.g., server + client builds simultaneously), distributed execution across machines, artifact caching to skip unchanged steps, and pluggable VCS support (Git, Perforce, Plastic SCM). A VCS-agnostic alternative to Horde for studios that don't want the Perforce lock-in. This is where Ludus and UET would converge most — UET's core strength is dynamic BuildGraph generation. Ludus's approach would differ: deployment-aware DAGs (build + containerize + deploy as graph nodes), AI-driven graph optimization via MCP, and Git-native rather than Perforce-centric. Competing on pure BuildGraph complexity is low-ROI; the value is in extending the graph through deployment.
- **Studio infrastructure provisioning** — Potentially a separate project that provisions game studio infrastructure on AWS (Perforce, CI/CD build farms, derived data cache, virtual workstations) as composable, pluggable modules that integrate with Ludus. AWS's [cloud-game-development-toolkit](https://github.com/aws-games/cloud-game-development-toolkit) (94 stars, Terraform, MIT-0) covers this space with modules for Perforce, Horde, Jenkins, TeamCity, Cloud DDC, and VDI — but is Perforce-centric and tightly coupled to Terraform. A Ludus-ecosystem alternative could be Git-native, engine-agnostic, and composable with the Ludus pipeline (e.g., `ludus deploy horde` or a separate CLI that provisions infrastructure Ludus can target). Decision point: integrate with the existing toolkit, wrap it, or build from scratch. Parked for now — revisit once pluggable deployment targets and BuildGraph are done.
- **WSL2 support** — OS prereq check update, `.wslconfig` memory guidance, Linux filesystem for I/O performance
- **macOS support** (stretch goal) — Mac-specific engine scripts (Setup.command, Xcode), cross-compilation strategy
- **Epic Launcher content automation** — Detect `legendary` CLI on Linux as alternative to Epic Games Launcher
