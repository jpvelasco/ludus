# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ludus is a Go CLI tool that automates the end-to-end pipeline for deploying Unreal Engine 5 Lyra dedicated servers to AWS GameLift Containers. It orchestrates: UE5 source builds → Lyra server compilation → Docker containerization → ECR push → GameLift fleet deployment.

## Build & Run Commands

```bash
go build -v                  # Build the ludus binary
go build -o ludus -v         # Build with explicit output name
go mod tidy                  # Clean up module dependencies
go vet ./...                 # Static analysis
go test ./...                # Run all tests (none exist yet)
go test -v ./internal/runner # Run tests for a single package
```

Run the CLI after building:
```bash
./ludus --help
./ludus init --verbose
./ludus run --dry-run
```

## Architecture

### Entry point

`main.go` → `root.Execute()` → Cobra command dispatch. The root command's `PersistentPreRunE` loads config via `config.Load()` into `globals.Cfg` before any subcommand runs.

### Command layer (`cmd/`)

Each subcommand lives in its own package under `cmd/` and exports a `Cmd *cobra.Command` variable. All commands are registered in `cmd/root/root.go` via `rootCmd.AddCommand()`.

Command hierarchy:
```
ludus init
ludus engine [build|setup]
ludus lyra [build|integrate-gamelift]
ludus container [build|push]
ludus deploy [fleet|stack|session|destroy]
ludus status
ludus run                    # full pipeline (6 stages)
```

Global persistent flags (`cmd/root/root.go`): `--config`, `--verbose/-v`, `--json`, `--dry-run`.

Global mutable state lives in `cmd/globals/globals.go`: `Cfg`, `Verbose`, `JSONOutput`, `DryRun`.

### Implementation layer (`internal/`)

All business logic is in `internal/` (unexported to consumers):

- **`config`** — `Config` struct with typed sub-structs (`EngineConfig`, `LyraConfig`, `ContainerConfig`, `GameLiftConfig`, `AWSConfig`). `Defaults()` returns sensible defaults. `Load()` reads `ludus.yaml` via Viper, expands relative paths, gracefully returns defaults if file is missing.
- **`runner`** — Shell command executor. `Run()` and `RunInDir()` use `exec.CommandContext`. Supports `Verbose` (prints `+ command` before running) and `DryRun` (prints without executing) modes. Streams stdout/stderr.
- **`prereq`** — `Checker` with `RunAll()` returning `[]CheckResult`. Validates OS, Lyra Content (downloaded from Epic Launcher Marketplace), Docker, AWS CLI, Git, Go, disk space (100 GB), RAM (16 GB).
- **`engine`** — `Builder` for UE5 compilation (Setup.sh, GenerateProjectFiles.sh, make with job limiting). Auto-detects max jobs from RAM (8 GB per job).
- **`lyra`** — `Builder` for Lyra server packaging via RunUAT BuildCookRun. Auto-detects `Lyra.uproject` from engine Samples directory. Pre-build fixups: writes `Directory.Build.props` (NuGetAuditLevel=critical) to work around Magick.NET CVEs in Epic's Gauntlet, and ensures `DefaultServerTarget=LyraServer` in DefaultEngine.ini for multi-target disambiguation.
- **`container`** — `Builder` for Dockerfile generation (Amazon Linux 2023, non-root user), `docker build`, and ECR push (login + tag + push).
- **`gamelift`** — `Deployer` for AWS GameLift via SDK v2. Creates container group definitions, IAM roles, fleets. Polls with 15s intervals / 30min timeout. Tags resources with `ludus:managed` and `ludus:fleet-name`. `Destroy()` tears down in reverse order, tolerating not-found errors.

### Patterns

- **Builder pattern**: Each major operation has a `Builder`/`Deployer` type with `New*(opts)` constructor, operation methods, and structured result types (`BuildResult`, `FleetStatus`).
- **Context threading**: All builders/deployers accept `context.Context` for cancellation and timeouts.
- **Runner abstraction**: Commands never call `exec.Command` directly — they use `runner.Runner` which handles verbose/dry-run modes uniformly.

## Configuration

Config template: `ludus.example.yaml`. User config: `ludus.yaml` (gitignored). Key settings: engine source path, max compile jobs (0 = auto-detect from RAM), server map (`L_Expanse`), server port (7777 UDP), GameLift instance type (`c6i.large`), container group name, AWS region/account.

## Key Domain Context

- UE5 must be built from source (Epic launcher builds can't produce dedicated server targets)
- Lyra Content assets are NOT in the GitHub source repo — must be downloaded from the Epic Games Launcher Marketplace ("Lyra Starter Game") and copied into the engine's `Samples/Games/Lyra/Content/` directory
- RAM is critical — UE5 linking can spike 8+ GB per job; `maxJobs` controls parallelism to prevent OOM
- UE 5.6 Lyra has multiple server targets (LyraServer, LyraServerEOS, LyraServerSteam, LyraServerSteamEOS) — `DefaultServerTarget=LyraServer` must be set in DefaultEngine.ini
- UE 5.6's Gauntlet test framework bundles Magick.NET 14.7.0 with known CVEs; combined with TreatWarningsAsErrors, AutomationTool script modules fail to compile without `NuGetAuditLevel=critical` in a Directory.Build.props
- GameLift integration has two approaches: Go SDK wrapper (no Lyra code changes, default) and direct C++ SDK integration (`ludus lyra integrate-gamelift`)
- Container must run as non-root user (Unreal server requirement)
- Linux x86_64 only (matches GameLift Containers requirement)

## Dependencies

Go 1.23.5, Cobra (CLI), Viper (config/YAML), AWS SDK for Go v2 (GameLift, IAM, config, credentials, STS/SSO for auth).

## Not Yet Implemented

- `ludus lyra integrate-gamelift` — C++ GameLift SDK patching into Lyra source
- `ludus deploy stack` — CloudFormation-based deployment

## Roadmap / Future Features

- **WSL2 support** — Pipeline should work largely as-is on WSL2; needs OS prereq check update, `.wslconfig` memory guidance, and documentation around keeping source on the Linux filesystem for I/O performance
- **macOS support** (stretch goal) — Engine builder needs Mac-specific scripts (Setup.command, Xcode), cross-compilation or Docker-based Linux server build strategy since GameLift requires Linux x86_64
- **Epic Launcher content automation** — Automate or guide Lyra Content download (e.g., detect `legendary` CLI on Linux as alternative to Epic Games Launcher)
