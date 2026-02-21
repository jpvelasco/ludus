# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ludus is a Go CLI tool that automates the end-to-end pipeline for deploying Unreal Engine 5 Lyra dedicated servers to AWS GameLift Containers. It orchestrates: UE5 source builds → Lyra server compilation → Docker containerization → ECR push → GameLift fleet deployment.

## Build & Run Commands

```bash
go build -o ludus -v         # Build (Linux/macOS)
go build -o ludus.exe -v .   # Build (Windows)
go mod tidy                  # Clean up module dependencies
go vet ./...                 # Static analysis
go test ./...                # Run all tests (none exist yet)
go test -v ./internal/runner # Run tests for a single package
```

Run the CLI after building:
```bash
./ludus --help
./ludus init --verbose       # Validate prerequisites
./ludus run --dry-run        # Full pipeline dry run
./ludus run --verbose --skip-engine  # Skip engine build stage
```

## Architecture

### Entry point

`main.go` → `root.Execute()` → Cobra command dispatch. The root command's `PersistentPreRunE` loads config via `config.Load()` into `globals.Cfg` before any subcommand runs.

### Command layer (`cmd/`)

Each subcommand lives in its own package under `cmd/` and exports a `Cmd *cobra.Command` variable. All commands are registered in `cmd/root/root.go` via `rootCmd.AddCommand()`.

Command hierarchy:
```
ludus init
ludus engine [build|setup]         --jobs/-j (0=auto)
ludus lyra [build|client|integrate-gamelift]  --skip-cook, --platform (Linux|Win64)
ludus container [build|push]       --tag/-t, --no-cache
ludus deploy [fleet|stack|session|destroy]  --region, --instance-type, --fleet-name
ludus connect                      --address (ip:port override)
ludus status                       # checks: engine source/build, lyra build, client build, container image, fleet, session
ludus run                          # full pipeline (6+ stages)
  --skip-engine, --skip-lyra, --skip-container, --skip-deploy, --with-client
```

Global persistent flags (`cmd/root/root.go`): `--config`, `--verbose/-v`, `--json`, `--dry-run`.

Global mutable state lives in `cmd/globals/globals.go`: `Cfg`, `Verbose`, `JSONOutput`, `DryRun`.

### Implementation layer (`internal/`)

All business logic is in `internal/` (unexported to consumers):

- **`config`** — `Config` struct with typed sub-structs (`EngineConfig`, `LyraConfig`, `ContainerConfig`, `GameLiftConfig`, `AWSConfig`). `Defaults()` returns sensible defaults. `Load()` reads `ludus.yaml` via Viper, expands relative paths, gracefully returns defaults if file is missing.
- **`runner`** — Shell command executor. `Run()` and `RunInDir()` use `exec.CommandContext`. Supports `Verbose` (prints `+ command` before running) and `DryRun` (prints without executing) modes. Streams stdout/stderr.
- **`prereq`** — `Checker` with `RunAll()` returning `[]CheckResult`. Validates: OS, engine source, Lyra Content, Docker, AWS CLI, Git, Go, disk space (100 GB), RAM (16 GB). Disk and memory checks are platform-specific (build-tagged files).
- **`engine`** — `Builder` for UE5 compilation. Linux: Setup.sh, GenerateProjectFiles.sh, make. Windows: Setup.bat, GenerateProjectFiles.bat, Build.bat. Targets: `ShaderCompileWorker` and `UnrealEditor` only (LyraServer is built via RunUAT in the lyra stage). Auto-detects max jobs from RAM (8 GB per job).
- **`lyra`** — `Builder` for Lyra packaging via RunUAT BuildCookRun. Cross-platform: `resolveRunUAT()` selects `cmd /c RunUAT.bat` (Windows) or `bash RunUAT.sh` (Linux). Uses relative script paths to avoid spaces-in-path issues with `cmd /c`. Path arguments are quoted (`-project="..."`) for the same reason. Pre-build fixups: writes `Directory.Build.props` (`NuGetAuditLevel=critical`) and ensures `DefaultServerTarget=LyraServer` in DefaultEngine.ini. `BuildClient()` supports `--platform` flag (Linux, Win64).
- **`container`** — `Builder` for Dockerfile generation (Amazon Linux 2023, non-root user), `docker build`, and ECR push (login + tag + push).
- **`gamelift`** — `Deployer` for AWS GameLift via SDK v2. Creates container group definitions, IAM roles, fleets. Polls with 15s intervals / 30min timeout. Tags resources with `ludus:managed` and `ludus:fleet-name`. `Destroy()` tears down in reverse order, tolerating not-found errors. `CreateGameSession` returns `*GameSessionInfo` (SessionID, IPAddress, Port). `DescribeGameSession` checks session liveness.
- **`state`** — Persistent state in `.ludus/state.json`. Tracks fleet (ID, status), session (ID, IP, port), and client build (binary path, platform, output dir). Read-modify-write via `Load()`/`Save()` with typed update helpers (`UpdateFleet`, `UpdateSession`, `UpdateClient`, `ClearSession`, `ClearFleet`).

### Platform-specific code

Four files use `//go:build` tags for platform-specific implementations:

- `internal/prereq/checker_windows.go` / `checker_unix.go` — Disk space (Windows: `GetDiskFreeSpaceExW`, Unix: `syscall.Statfs`) and memory checks (Windows: `GlobalMemoryStatusEx`, Unix: `/proc/meminfo`)
- `cmd/connect/launch_windows.go` / `launch_unix.go` — Client launch (Windows: `os/exec.Command` to start as child process, Unix: `syscall.Exec` to replace current process)
- `cmd/status/status.go` — Uses `runtime.GOOS` to check for `Setup.bat`/`UnrealEditor.exe` (Windows) vs `Setup.sh`/`UnrealEditor` (Linux)

### Patterns

- **Builder pattern**: Each major operation has a `Builder`/`Deployer` type with `New*(opts)` constructor, operation methods, and structured result types (`BuildResult`, `FleetStatus`).
- **Context threading**: All builders/deployers accept `context.Context` for cancellation and timeouts.
- **Runner abstraction**: Commands never call `exec.Command` directly — they use `runner.Runner` which handles verbose/dry-run modes uniformly.
- **Config override**: Deploy subcommands accept `--region`, `--instance-type`, `--fleet-name` flags that override `ludus.yaml` values.
- **State persistence**: Deploy and client-build commands write to `.ludus/state.json` so downstream commands (`connect`, `status`) can resolve fleet/session/client info without re-querying AWS.

## Configuration

Config template: `ludus.example.yaml`. User config: `ludus.yaml` (gitignored). Key settings: engine source path, max compile jobs (0 = auto-detect from RAM), server map (`L_Expanse`), server port (7777 UDP), GameLift instance type (`c6i.large`), container group name, AWS region/account.

## Key Domain Context

- UE5 must be built from source (Epic launcher builds can't produce dedicated server targets)
- Lyra Content assets are NOT in the GitHub source repo — must be downloaded from the Epic Games Launcher Marketplace ("Lyra Starter Game") and the **entire project** must be overlaid onto the engine's `Samples/Games/Lyra/` directory. This includes both the top-level `Content/` directory AND `Plugins/GameFeatures/*/Content/` directories (ShooterCore, ShooterExplorer, ShooterMaps, ShooterTests, TopDownArena each have their own Content folder with GameFeatureData assets). Missing plugin content causes cook failures (ExitCode=25, "GameFeatureData is missing"). The Epic Games Launcher does not run on Linux; Windows or macOS required for this one-time download.
- RAM is critical — UE5 linking can spike 8+ GB per job; `maxJobs` controls parallelism to prevent OOM
- UE 5.6 Lyra has multiple server targets (LyraServer, LyraServerEOS, LyraServerSteam, LyraServerSteamEOS) — `DefaultServerTarget=LyraServer` must be set in DefaultEngine.ini
- UE 5.6's Gauntlet test framework directly depends on Magick.NET 14.7.0 with known CVEs; combined with TreatWarningsAsErrors, AutomationTool script modules fail to compile without `NuGetAuditLevel=critical` in a Directory.Build.props at `Engine/Source/Programs/`
- GameLift integration has two approaches: Go SDK wrapper (no Lyra code changes, default) and direct C++ SDK integration (`ludus lyra integrate-gamelift`)
- Container must run as non-root user (Unreal server requirement)
- Server builds are Linux x86_64 only (matches GameLift Containers requirement)
- Client builds support Linux and Win64; native Win64 builds work if UE5 is built from source on Windows
- `ludus connect` launches the client directly on both platforms (Windows: `os/exec` child process, Linux: `syscall.Exec` process replacement). On Linux with a Win64 client, it prints copy/run instructions instead.
- UE5 content cooking requires 24+ GB RAM; 32 GB recommended. On Ubuntu, `systemd-oomd` kills the cook process at 50% memory pressure — disable it before building (`sudo systemctl disable --now systemd-oomd systemd-oomd.socket`)
- UE 5.6.1 on Windows requires specific source patches and toolchain versions — see `UE_SOURCE_PATCHES.md` for details (INITGUID fix for NNERuntimeORT on SDK >= 26100, MSVC 14.38 toolchain requirement)

## Dependencies

Go 1.23.5, Cobra v1.10.2 (CLI), Viper v1.21.0 (config/YAML), AWS SDK for Go v2 (GameLift, IAM, config, credentials, STS/SSO for auth).

## Cross-Platform Notes

The server pipeline (engine build → container → deploy) is Linux-only. The client build and connect commands work on both Linux and Windows.

On Windows:
1. `go build -o ludus.exe -v .`
2. Configure `ludus.yaml` with `engine.sourcePath` pointing to the Windows UE5 source
3. `ludus.exe lyra client --platform Win64 --verbose` — builds the Win64 game client
4. `ludus.exe deploy session` — creates a game session (or copy `.ludus/state.json` from the Linux machine)
5. `ludus.exe connect` — launches the client directly and connects to the server

Windows-specific prerequisites not yet automated in `ludus init`:
- MSVC 14.38 toolchain (VS 2025/2026 ships 14.50+ which triggers UE build errors)
- VS workloads: "Desktop development with C++", "Game development with C++", Windows SDK
- UE 5.6.1 source patch: INITGUID in NNERuntimeORT.Build.cs (see `UE_SOURCE_PATCHES.md`)
- `BuildConfiguration.xml` at `%APPDATA%\Unreal Engine\UnrealBuildTool\` to pin MSVC version

## Not Yet Implemented

- `ludus lyra integrate-gamelift` — C++ GameLift SDK patching into Lyra source
- `ludus deploy stack` — CloudFormation-based deployment
- Enhanced `ludus init` for Windows — auto-detect MSVC toolchain, verify VS workloads, check Lyra content

## Validated End-to-End

- Linux: Engine → Lyra server → container → ECR → GameLift fleet → game sessions (UDP connectivity confirmed)
- Windows: Win64 client built → connected to GameLift fleet → played on live Linux server container

## Roadmap

- **WSL2 support** — OS prereq check update, `.wslconfig` memory guidance, Linux filesystem for I/O performance
- **macOS support** (stretch goal) — Mac-specific engine scripts (Setup.command, Xcode), cross-compilation strategy
- **Epic Launcher content automation** — Detect `legendary` CLI on Linux as alternative to Epic Games Launcher
