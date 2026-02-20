# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ludus is a Go CLI tool that automates the end-to-end pipeline for deploying Unreal Engine 5 Lyra dedicated servers to AWS GameLift Containers. It orchestrates: UE5 source builds → Lyra server compilation → Docker containerization → ECR push → GameLift fleet deployment.

## Build & Run Commands

```bash
go build -o ludus -v         # Build with explicit output name
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
- **`prereq`** — `Checker` with `RunAll()` returning `[]CheckResult`. Validates: OS, engine source, Lyra Content (downloaded from Epic Launcher Marketplace), Docker, AWS CLI, Git, Go, disk space (100 GB), RAM (16 GB).
- **`engine`** — `Builder` for UE5 compilation (Setup.sh, GenerateProjectFiles.sh, make). Targets: `ShaderCompileWorker` and `UnrealEditor` only (LyraServer is built via RunUAT in the lyra stage). Auto-detects max jobs from RAM (8 GB per job).
- **`lyra`** — `Builder` for Lyra server packaging via RunUAT BuildCookRun. Auto-detects `Lyra.uproject` from engine Samples directory. Pre-build fixups: writes `Directory.Build.props` (`NuGetAuditLevel=critical`) to work around Magick.NET CVEs in Epic's Gauntlet, and ensures `DefaultServerTarget=LyraServer` in DefaultEngine.ini for multi-target disambiguation. `BuildClient()` supports `--platform` flag (Linux native, Win64 cross-compile).
- **`container`** — `Builder` for Dockerfile generation (Amazon Linux 2023, non-root user), `docker build`, and ECR push (login + tag + push).
- **`gamelift`** — `Deployer` for AWS GameLift via SDK v2. Creates container group definitions, IAM roles, fleets. Polls with 15s intervals / 30min timeout. Tags resources with `ludus:managed` and `ludus:fleet-name`. `Destroy()` tears down in reverse order, tolerating not-found errors. `CreateGameSession` returns `*GameSessionInfo` (SessionID, IPAddress, Port). `DescribeGameSession` checks session liveness.
- **`state`** — Persistent state in `.ludus/state.json`. Tracks fleet (ID, status), session (ID, IP, port), and client build (binary path, platform, output dir). Read-modify-write via `Load()`/`Save()` with typed update helpers (`UpdateFleet`, `UpdateSession`, `UpdateClient`, `ClearSession`, `ClearFleet`).

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
- Client builds support Linux and Win64; Win64 cross-compile from Linux requires the MSVC cross-compile toolchain (not bundled); native Win64 builds work if UE5 is built from source on Windows
- `ludus connect` uses `syscall.Exec` on Linux to replace itself with the game client; for Win64 clients it prints copy/run instructions instead
- UE5 content cooking requires 24+ GB RAM; 32 GB recommended. On Ubuntu, `systemd-oomd` kills the cook process at 50% memory pressure — disable it before building (`sudo systemctl disable --now systemd-oomd systemd-oomd.socket`)

## Dependencies

Go 1.23.5, Cobra v1.10.2 (CLI), Viper v1.21.0 (config/YAML), AWS SDK for Go v2 (GameLift, IAM, config, credentials, STS/SSO for auth).

## Windows Client Development

To build and test a Win64 Lyra client (for connecting to the Linux GameLift server):

1. Clone this repo on a Windows machine with UE5 built from source
2. `go build -o ludus.exe -v .`
3. Configure `ludus.yaml` with `engine.sourcePath` pointing to the Windows UE5 source
4. `ludus.exe lyra client --platform Win64 --verbose` — builds the Win64 game client
5. `ludus.exe deploy session` — creates a game session (or copy `.ludus/state.json` from the Linux machine)
6. `ludus.exe connect` — prints the launch command with the server address

On Windows, `ludus connect` prints the launch command rather than exec'ing directly (no `syscall.Exec` equivalent). Run the printed command to launch the client.

The server pipeline (engine build, container, deploy) remains Linux-only. Only the client build and connect commands are relevant on Windows.

## Not Yet Implemented

- `ludus lyra integrate-gamelift` — C++ GameLift SDK patching into Lyra source
- `ludus deploy stack` — CloudFormation-based deployment

## Current Deployment State

The full server pipeline has been validated end-to-end on Linux:
- Engine built from source (UE 5.6.1)
- Lyra server packaged, containerized, pushed to ECR
- GameLift fleet ACTIVE, game sessions reachable (UDP connectivity confirmed)
- Linux client built and state persisted

Remaining validation: graphical client connection test (requires a machine with Vulkan-capable GPU — the Linux VM uses VMware SVGA which lacks SM6 Vulkan support). Win64 native build on a Windows host with a real GPU is the recommended path.

## Windows Prerequisites Issues (Discovered)

These are known issues encountered when running on Windows that need to be addressed in `ludus init` or dedicated prereq checks:

1. **MSVC Toolchain Version**: UE 5.6.1 requires MSVC 14.38 (VS 2022 v143 toolset). If the user has VS 2025/2026 (MSVC 14.50+), the newer compiler triggers warnings (C4756 overflow in constant arithmetic, C4458 declaration hides class member) in AnimNextAnimGraph, RigLogicLib, and MetaHuman plugins that UE promotes to errors via `/WX`. **Fix**: Install the 14.38 toolchain via VS Installer (`Microsoft.VisualStudio.Component.VC.14.38.17.8.x86.x64`) and set `<CompilerVersion>14.38.33130</CompilerVersion>` in `%APPDATA%\Unreal Engine\UnrealBuildTool\BuildConfiguration.xml`. Ludus should detect the wrong toolchain version and either install the correct one (with permission) or create the BuildConfiguration.xml automatically.

2. **Lyra Content Assets**: Not included in UE GitHub source. Must be downloaded from Epic Games Launcher Marketplace ("Lyra Starter Game") and the **entire downloaded project** must be overlaid onto `Engine/Samples/Games/Lyra/`. Critical: this includes not just the top-level `Content/` directory but also `Plugins/GameFeatures/*/Content/` directories. Each GameFeature plugin (ShooterCore, ShooterExplorer, ShooterMaps, ShooterTests, TopDownArena) has its own Content folder containing GameFeatureData `.uasset` files. If these are missing, the cook phase fails with ExitCode=25 ("GameFeatureData is missing"). Ludus should detect missing content (both main and plugin) and provide clear instructions or automate the copy from a user-specified path.

3. **AWS CLI PATH**: After MSI installation on Windows, the current shell session may not have the updated PATH. Ludus should use the full path (`C:\Program Files\Amazon\AWSCLIV2\aws.exe`) as a fallback or prompt the user to restart their terminal.

4. **RunUAT Script**: Windows uses `RunUAT.bat` (invoked via `cmd /c`) while Linux uses `RunUAT.sh` (invoked via `bash`). Already handled in code via `resolveRunUAT()`.

5. **Windows Power Settings**: Long builds (6+ hours for full engine) require the system to stay awake. Ludus should warn if power plan allows sleep during builds, or recommend high-performance mode.

6. **Visual Studio Installation**: UE 5.6.1 needs specific VS workloads: "Desktop development with C++", "Game development with C++", and the Windows 10/11 SDK. Ludus should verify these are installed.

## Roadmap / Future Features

- **Enhanced `ludus init` for Windows** — Auto-detect MSVC toolchain version, install correct toolchain with user permission, create BuildConfiguration.xml, verify VS workloads, check Lyra content
- **Windows native client build** — Build Win64 client on Windows host with GPU for full graphical connection test to GameLift server
- **WSL2 support** — Pipeline should work largely as-is on WSL2; needs OS prereq check update, `.wslconfig` memory guidance, and documentation around keeping source on the Linux filesystem for I/O performance
- **macOS support** (stretch goal) — Engine builder needs Mac-specific scripts (Setup.command, Xcode), cross-compilation or Docker-based Linux server build strategy since GameLift requires Linux x86_64
- **Epic Launcher content automation** — Automate or guide Lyra Content download (e.g., detect `legendary` CLI on Linux as alternative to Epic Games Launcher)
