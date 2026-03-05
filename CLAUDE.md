# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ludus is a Go CLI tool that automates the end-to-end pipeline for deploying Unreal Engine 5 dedicated servers to AWS GameLift. It orchestrates: UE5 source builds → game server compilation → deployment (via Docker containers, Managed EC2, or local Anywhere). Server builds can be cross-compiled from Windows. While Lyra (Epic's sample game) is the default project, Ludus supports any UE5 game with dedicated server targets.

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
./ludus init --fix           # Auto-fix issues where possible (Windows: VS components, BuildConfiguration.xml, NNERuntimeORT patch, cross-compile toolchain)
./ludus run --dry-run        # Full pipeline dry run
./ludus run --verbose --skip-engine  # Skip engine build stage
```

## Architecture

### Entry point

`main.go` → `root.Execute()` → Cobra command dispatch. The root command's `PersistentPreRunE` loads config via `config.Load()` into `globals.Cfg` before any subcommand runs, then auto-detects engine version from `Engine/Build/Build.version` if `cfg.Engine.SourcePath` is set but `cfg.Engine.Version` is empty. `SilenceUsage: true` is set on `rootCmd` so Cobra only prints the error message on failure, not the full usage text.

### Command layer (`cmd/`)

Each subcommand lives in its own package under `cmd/` and exports a `Cmd *cobra.Command` variable. All commands are registered in `cmd/root/root.go` via `rootCmd.AddCommand()`.

Command hierarchy:
```
ludus setup                         # interactive wizard — scans for engines, detects version, writes ludus.yaml
ludus init                          --fix (auto-remediate on Windows)
ludus config [set|get]             set/get config values in ludus.yaml via dot-notation (profile-aware)
ludus engine [build|setup|push]    --jobs/-j (0=auto), --backend (native|docker), --no-cache, --base-image
ludus game [build|client]          --skip-cook, --platform (Linux|Win64), --backend (native|docker), --no-cache, --config (Development|Shipping)
ludus container [build|push]       --tag/-t, --no-cache
ludus deploy [fleet|stack|anywhere|ec2|session|destroy]  --target, --region, --instance-type, --fleet-name, --stack-name, --ip, --with-session, destroy --all
ludus connect                      --address (ip:port override)
ludus doctor                       # deep diagnostics: toolchain, stale artifacts, state, cache, disk, AWS creds, Docker, git
ludus status                       # checks: engine source/build, game build, client build, container image, fleet, session
ludus run                          # full pipeline (7+ stages)
  --skip-engine, --skip-game, --skip-container, --skip-deploy, --with-client, --with-session, --backend (native|docker), --no-cache
ludus mcp                          # start MCP server (stdio JSON-RPC)
ludus ci init                      # generate GitHub Actions workflow
  --output/-o, --enable-push, --enable-pr
ludus ci runner [install|status|uninstall]  # self-hosted runner management
  --dir, --labels, --name, --repo, --service, --delete
```

Global persistent flags (`cmd/root/root.go`): `--config`, `--verbose/-v`, `--json`, `--dry-run`, `--profile`.

Global mutable state lives in `cmd/globals/globals.go`: `Cfg`, `Verbose`, `JSONOutput`, `DryRun`, `Profile`.

### MCP server (`cmd/mcp/`)

`ludus mcp` starts a Model Context Protocol server over stdio (JSON-RPC). AI agents use the exposed tools to orchestrate the full pipeline. The server uses the official Go MCP SDK (`github.com/modelcontextprotocol/go-sdk`).

**Stdout protection**: MCP uses stdout for JSON-RPC transport. At startup, real stdout is saved for the MCP transport, then `os.Stdout` is redirected to `os.Stderr`. Each tool call uses `withCapture()` to capture output from internal packages.

**Tools** (16 total): `ludus_init`, `ludus_status`, `ludus_engine_setup`, `ludus_engine_build`, `ludus_engine_push`, `ludus_game_build`, `ludus_game_client`, `ludus_container_build`, `ludus_container_push`, `ludus_deploy_fleet`, `ludus_deploy_stack`, `ludus_deploy_anywhere`, `ludus_deploy_ec2`, `ludus_deploy_session`, `ludus_deploy_destroy`, `ludus_connect_info`. Deploy tools (`fleet`, `stack`, `anywhere`, `ec2`) accept `with_session` to auto-create a game session after deployment.

**Error convention**: Operational errors (build failed, AWS error) return `CallToolResult{IsError: true}` with JSON content. Go errors are reserved for protocol-level failures.

**Files**: `mcp.go` (server setup, Cobra command), `register.go` (tool registration), `capture.go` (stdout/stderr capture), `helpers.go` (shared utilities), `tools_*.go` (one file per domain).

MCP client configuration example:
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

### CI integration (`cmd/ci/`, `internal/ci/`)

`ludus ci init` generates a GitHub Actions workflow file for the UE5 server pipeline. The workflow uses `workflow_dispatch` with per-stage skip flags (engine, game, container, deploy) and a dry-run option. Push/PR triggers are commented out by default; `--enable-push`/`--enable-pr` flags uncomment them. The workflow assumes a self-hosted runner with UE5 already built and `ludus.yaml` present on the machine.

`ludus ci runner install|status|uninstall` manages the GitHub Actions self-hosted runner agent on the current Linux machine. It uses the `gh` CLI for registration/removal tokens and auto-detects the GitHub repo from git remotes. The `--service` flag installs the runner as a systemd service.

**Files**: `cmd/ci/ci.go` (parent command + init), `cmd/ci/runner.go` (runner subcommands), `internal/ci/workflow.go` (workflow template generation), `internal/ci/runner.go` (runner agent management).

### Implementation layer (`internal/`)

All business logic is in `internal/` (unexported to consumers):

- **`config`** — `Config` struct with typed sub-structs (`EngineConfig`, `GameConfig`, `ContainerConfig`, `DeployConfig`, `GameLiftConfig`, `EC2FleetConfig`, `AnywhereConfig`, `AWSConfig`, `CIConfig`). `AnywhereConfig` includes `LocationName` (default `"custom-ludus-dev"`), `IPAddress` (default empty for auto-detect), and `AWSProfile` (default `"default"`). `EngineConfig` includes `Backend` (`"native"` or `"docker"`), `DockerImage` (pre-built engine image URI), `DockerImageName` (local image name, default `"ludus-engine"`), and `DockerBaseImage` (base Docker image, default `"ubuntu:22.04"`, supports apt-get and dnf-based images). `GameConfig` includes `ProjectName`, `ContentSourcePath` (path to downloaded game content for auto-overlay), `ServerTarget`, `ClientTarget`, `GameTarget` fields with resolver methods (`ResolvedServerTarget()`, etc.) that default to `ProjectName+"Server"` etc. `AWSConfig` includes `Tags map[string]string` for configurable resource tagging (default: `ManagedBy: ludus`). `CIConfig` holds `WorkflowPath`, `RunnerDir`, and `RunnerLabels` for CI workflow generation and runner management. `Defaults()` returns sensible defaults with `ProjectName: "Lyra"`, `Backend: "native"`. `Load()` reads `ludus.yaml` via Viper, expands relative paths, gracefully returns defaults if file is missing. Backward compat: if `lyra:` key present but no `game:` key, migrates and prints deprecation warning to stderr.
- **`runner`** — Shell command executor. `Run()` and `RunInDir()` use `exec.CommandContext`. `RunOutput()` captures stdout as bytes instead of streaming (used by CI runner installer for `gh api` token output). Supports `Verbose` (prints `+ command` before running) and `DryRun` (prints without executing) modes. Streams stdout/stderr. `Env []string` field allows setting extra environment variables on child processes (merged on top of parent env, overriding matching keys).
- **`ci`** — `GenerateWorkflow(opts)` returns GitHub Actions YAML content using `fmt.Sprintf` (matches Dockerfile generation pattern). `WriteWorkflow(path, content)` creates parent dirs and writes the file. `RunnerInstaller` manages the self-hosted runner agent lifecycle: `Install()` (download, extract, configure, optionally install systemd service), `Status()` (check systemd/process), `Uninstall()` (deregister, optionally delete). `ParseRepoFromRemote()` extracts `owner/repo` from SSH or HTTPS git URLs.
- **`prereq`** — `Checker` with `RunAll()` returning `[]CheckResult`. Cross-platform checks: OS (linux/windows), engine source, toolchain (via `toolchain` package), game content (Lyra-specific with auto-discovery of downloaded content in `Documents/Unreal Projects/LyraStarterGame*`, or generic via `ContentValidationConfig`), server map (searches for `<serverMap>.umap` in Content/, warn-only), Docker (warn-only on Windows), AWS CLI, AWS credentials (`sts get-caller-identity`, warn-only), Git, Go, disk space (100 GB), RAM (16 GB). Lyra auto-discovery: `discoverLyraContent()` scans common download paths (Documents, OneDrive) and validates with `isLyraProject()` (checks for Lyra.uproject or Content/DefaultGameData.uasset); with `--fix`, auto-overlays discovered content. Windows-specific checks via `platformChecks()`: Visual Studio workloads/components (via vswhere), MSVC 14.38 toolchain config (`BuildConfiguration.xml`), Windows SDK version, NNERuntimeORT INITGUID patch, Linux cross-compile toolchain (auto-download/install with `--fix`). `CheckResult` has `Warning bool` for non-fatal issues. `Checker.Fix bool` gates auto-remediation (`--fix` flag). Disk, memory, and platform checks use build-tagged files.
- **`toolchain`** — Engine version detection and cross-compile toolchain validation. `ParseBuildVersion()` reads `Engine/Build/Build.version` JSON. `DetectEngineVersion()` tries Build.version first, falls back to config string. `LookupToolchain()` maps engine major.minor (5.4→v22/clang-16, 5.5→v23/clang-18, 5.6→v25/clang-18, 5.7→v26/clang-20) to `ToolchainSpec`. `ToolchainSpec` includes `InstallerURL` for Windows cross-compile toolchain auto-download. `CheckToolchain()` orchestrates detection + platform-specific search: Linux scans `Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/Linux_x64/` and `LINUX_MULTIARCH_ROOT`; Windows checks `LINUX_MULTIARCH_ROOT` only. No build tags — uses `runtime.GOOS` for platform branching.
- **`cache`** — Build caching for pipeline stages. `Cache` struct with `Load()`/`Save()` to `.ludus/cache.json`. Each stage has a key function (`EngineKey`, `GameServerKey`, `GameClientKey`, `ContainerKey`) that hashes relevant inputs (git HEAD, config values, file mtime/size). `IsHit()` checks if a stage's inputs match the cached hash. `Set()` records a new entry. Used by all build commands and the pipeline to skip unchanged stages. `--no-cache` flag bypasses the check.
- **`dockerbuild`** — Docker-based build backend. `EngineImageBuilder` builds UE5 inside Docker (`docker build` with generated Dockerfile) and pushes to ECR. `DockerGameBuilder` runs game server/client builds inside a pre-built engine container (`docker run` with volume mounts for output and optional project). `GenerateEngineDockerfile()` produces a Dockerfile from a configurable base image (default `ubuntu:22.04`, supports apt-get and dnf) with UE5 build prerequisites, configurable `MAX_JOBS` build arg. Build scripts handle the same workarounds as the native builder (NuGetAuditLevel, DefaultServerTarget).
- **`engine`** — `Builder` for UE5 compilation. Linux: Setup.sh, GenerateProjectFiles.sh, make. Windows: Setup.bat, GenerateProjectFiles.bat, Build.bat. Targets: `ShaderCompileWorker` and `UnrealEditor` only (game server is built via RunUAT in the game stage). Auto-detects max jobs from RAM (8 GB per job).
- **`game`** — `Builder` for UE5 game packaging via RunUAT BuildCookRun. Cross-platform: `resolveRunUAT()` selects `cmd /c RunUAT.bat` (Windows) or `bash RunUAT.sh` (Linux). Uses relative script paths to avoid spaces-in-path issues with `cmd /c`. Path arguments are quoted (`-project="..."`) for the same reason. Pre-build fixups: `applyNuGetAuditWorkaround()` sets `NuGetAuditLevel=critical` as an env var on the runner (avoids writing `Directory.Build.props` into engine source; version-gated to 5.6/unknown), and `ensureDefaultServerTarget()` configures DefaultEngine.ini (game project config, not engine source; skips gracefully if INI structure doesn't match). `BuildClient()` supports `--platform` flag (Linux, Win64). All target names (`-servertargetname`, binary paths) are config-driven via `BuildOptions`. `EngineVersion` in `BuildOptions` enables version-specific workarounds.
- **`container`** — `Builder` for Dockerfile generation (Amazon Linux 2023, non-root user), `docker build`, and ECR push (login + tag + push). Project name and server target are parameterized in generated Dockerfile and wrapper config.
- **`diagnose`** — Contextual error guidance for common failure modes. Table-driven pattern matching with three categories: `awsHints` (12 patterns — expired tokens, access denied, quota limits, missing credentials), `deployHints` (6 patterns — fleet errors, timeouts, conflicts, IP detection), `containerHints` (6 patterns — disk full, daemon not running, ECR auth, rate limits). Public functions: `AWSError(err, operation)`, `DeployError(err, target)`, `ContainerError(err, operation)`. Returns the original error wrapped with actionable `Suggestions:` section, or the original error unchanged if no patterns match.
- **`progress`** — Lightweight elapsed-time ticker for long-running operations. `Start(operation, interval)` spawns a goroutine that prints periodic `[elapsed] operation still running...` messages. `Stop()` terminates the ticker. Used in engine compile (2 min interval), game server build, and client build to reassure users during multi-hour builds with long silent periods.
- **`deploy`** — `Target` interface abstracting deployment backends, with `Capabilities` (what the target needs/supports), `Deploy()`, `Status()`, `Destroy()` methods. Optional `SessionManager` interface for targets that support game sessions. Shared types: `DeployInput`, `DeployResult`, `DeployStatus`, `SessionInfo`. Implementations are in `gamelift`, `stack`, `binary`, `anywhere`, and `ec2fleet` packages; target resolution lives in `cmd/globals/resolve.go`.
- **`gamelift`** — `Deployer` for AWS GameLift via SDK v2. Creates container group definitions, IAM roles, fleets. Polls with 15s intervals / 30min timeout. Uses shared `tags` package for resource tagging. `Destroy()` tears down in reverse order, tolerating not-found errors. `TargetAdapter` wraps `Deployer` to implement `deploy.Target` and `deploy.SessionManager`. `CreateGameSession` returns `*GameSessionInfo` (SessionID, IPAddress, Port). `DescribeGameSession` checks session liveness.
- **`stack`** — `StackDeployer` for CloudFormation-based deployment. `Deploy()` generates a CF template (IAM role, container group definition, container fleet), calls `CreateStack`/`UpdateStack`, and polls until complete. `Destroy()` calls `DeleteStack`. `TargetAdapter` wraps `StackDeployer` to implement `deploy.Target` and `deploy.SessionManager` (reads fleet ID from stack outputs for session management). Stack naming: `ludus-<fleet-name>` by default.
- **`tags`** — Centralized AWS resource tagging. `Build(cfg)` constructs the full tag set from `cfg.AWS.Tags`, auto-derives `Project` from `cfg.Game.ProjectName`, ensures `ManagedBy: ludus`. Conversion helpers: `ToGameLiftTags()`, `ToIAMTags()`, `ToCFNTags()`, `ToS3Tags()`, `ToTemplateTags()`. `Merge()` and `WithResourceName()` for tag composition.
- **`pricing`** — Instance-type pricing and guidance. `InstanceSpec` struct with type, category, vCPUs, memory, price, arch, and notes. `EstimateCost(instanceType)` returns hourly USD cost. `FormatEstimate(instanceType)` returns a human-readable cost string. `FormatGuidance(currentType, arch)` returns a curated comparison table (compute/general/memory categories, Graviton alternatives). `FormatSuggestion(currentType, arch)` returns a one-liner Graviton savings tip. Covers c5/c6i/c6g/c7g (compute), m5/m6i/m6g (general), r5/r6i (memory) families. Used by deploy commands, pipeline, and MCP tools.
- **`binary`** — `Exporter` implements `deploy.Target` for simple file export. `Deploy()` copies the server build directory to a configurable output dir via `cp -a`. `Status()` checks if the output dir exists and has files. `Destroy()` removes the output dir.
- **`anywhere`** — `Deployer` and `TargetAdapter` implement `deploy.Target` + `deploy.SessionManager` for GameLift Anywhere. `Deploy()` creates a custom location, Anywhere fleet, registers the local machine as a compute, builds the Game Server Wrapper, and launches the server locally. `CreateSession()` creates game sessions with the required `Location` parameter. `Destroy()` stops the server, deregisters compute, deletes fleet and location. State tracked in `AnywhereState` (PID, compute name, fleet/location ARNs). `DetectLocalIP()` auto-detects the machine's non-loopback IPv4.
- **`ec2fleet`** — `Deployer` and `TargetAdapter` implement `deploy.Target` + `deploy.SessionManager` for GameLift Managed EC2. `Deploy()` zips the server build with the Game Server Wrapper, uploads to S3, creates a GameLift Build, then creates an EC2 fleet with runtime configuration. No Docker required. `Destroy()` deletes fleet, build, S3 object, and IAM role. State tracked in `EC2FleetState` (FleetID, BuildID, S3Bucket, S3Key). Uses STS for auto-deriving S3 bucket name (`ludus-builds-<account-id>`).
- **`wrapper`** — Shared package for the Amazon GameLift Game Server Wrapper binary. `EnsureBinary()` clones the wrapper repo, builds it, and caches at `~/.cache/ludus/game-server-wrapper/`. Used by `container` (for Docker builds), `anywhere` (for local server launch), and `ec2fleet` (included in build zip).
- **`state`** — Persistent state with profile support. Default profile: `.ludus/state.json`. Named profiles: `.ludus/profiles/<name>.json`. `SetProfile(name)` activates a profile (called from root command's `PersistentPreRunE`). `Load()`/`Save()` automatically use the active profile. Tracks fleet, session, client build, deploy, engine image, anywhere, and EC2 fleet state. Read-modify-write via typed update helpers (`UpdateFleet`, `UpdateSession`, `UpdateClient`, `UpdateDeploy`, `UpdateEngineImage`, `UpdateEC2Fleet`, `ClearSession`, `ClearFleet`, `ClearEC2Fleet`). `ListProfiles()` returns all named profiles. `DeleteProfile(name)` removes a profile's state file.
- **`status`** — Extracted from `cmd/status/status.go`. `StageStatus` type and check functions (`CheckEngineSource`, `CheckEngineBuild`, `CheckServerBuild`, `CheckContainerImage`, `CheckClientBuild`, `CheckDeployTarget`, `CheckGameSession`). `CheckAll(ctx, cfg, target)` runs all checks and returns `[]StageStatus`. Used by both `cmd/status` (CLI display) and `cmd/mcp` (MCP tool).

### Platform-specific code

Build-tagged files use `//go:build` tags for platform-specific implementations:

- `internal/prereq/checker_windows.go` / `checker_unix.go` — Disk space (Windows: `GetDiskFreeSpaceExW`, Unix: `syscall.Statfs`), memory checks (Windows: `GlobalMemoryStatusEx`, Unix: `/proc/meminfo`), and `platformChecks()` dispatch (Windows: VS/MSVC/SDK/patch checks; Unix: no-op)
- `internal/anywhere/process_unix.go` / `process_windows.go` — Server process management (Unix: `Setpgid`, `SIGTERM`/`SIGKILL`, signal-0 probing; Windows: basic `Kill`/stub for cross-compilation)
- `cmd/connect/launch_windows.go` / `launch_unix.go` — Client launch (Windows: `os/exec.Command` to start as child process, Unix: `syscall.Exec` to replace current process)
- `cmd/status/status.go` — Uses `runtime.GOOS` to check for `Setup.bat`/`UnrealEditor.exe` (Windows) vs `Setup.sh`/`UnrealEditor` (Linux)

### Patterns

- **Builder pattern**: Each major operation has a `Builder`/`Deployer` type with `New*(opts)` constructor, operation methods, and structured result types (`BuildResult`, `FleetStatus`).
- **Context threading**: All builders/deployers accept `context.Context` for cancellation and timeouts.
- **Runner abstraction**: Commands never call `exec.Command` directly — they use `runner.Runner` which handles verbose/dry-run modes uniformly.
- **Pluggable targets**: Deployment is abstracted behind `deploy.Target` interface. `cmd/globals.ResolveTarget()` is the factory that creates the appropriate target based on config (`deploy.target` in `ludus.yaml`) or CLI flag (`--target`). Implementations: `gamelift` (container fleet), `stack` (CloudFormation), `binary` (file export), `anywhere` (local dev), `ec2fleet` (Managed EC2). The pipeline checks `target.Capabilities()` to skip container/push stages for targets that don't need them (binary, anywhere, and ec2 skip containers). GameLift-specific commands (`fleet`, `session`) still use the direct `gamelift.Deployer` when needed; generic commands (`destroy`, pipeline deploy) use the interface.
- **Config override**: Deploy subcommands accept `--target`, `--region`, `--instance-type`, `--fleet-name`, `--stack-name`, `--ip`, `--with-session` flags that override `ludus.yaml` values.
- **Auto-detect engine version**: `PersistentPreRunE` in `root.go` auto-populates `cfg.Engine.Version` from `Engine/Build/Build.version` when the source path is set but version is empty, so users don't need to set `engine.version` in `ludus.yaml`.
- **What's-next guidance**: Every command prints a `Next:` hint after success output, guiding users to the next pipeline step. Deploy hints are gated on `!withSession` (session already created). Game build hints are target-aware (container build for gamelift/stack, deploy for ec2/anywhere/binary).
- **State persistence**: Deploy and client-build commands write to `.ludus/state.json` (or `.ludus/profiles/<profile>.json`) so downstream commands (`connect`, `status`) can resolve fleet/session/client info without re-querying AWS.
- **Profiles**: `--profile <name>` isolates both config and state per workflow. Config loading tries `ludus-<profile>.yaml` first, falls back to `ludus.yaml`. State uses `.ludus/profiles/<name>.json`. `ludus --profile ue57-ec2 setup` creates a profile-specific config; all subsequent commands with `--profile ue57-ec2` use it. Default (no profile) preserves existing behavior.
- **Config-driven targets**: Game project name, server target, client target, and game target are all configurable via `ludus.yaml`. Defaults derive from `ProjectName` (e.g., `ProjectName+"Server"` for `ServerTarget`). Lyra-specific behavior (auto-detection of project path, content validation with plugin dirs) is preserved as a fallback when `ProjectName == "Lyra"`.
- **Cost estimates and instance guidance**: Deploy commands (`fleet`, `stack`, `ec2`) and the pipeline print estimated hourly/monthly cost and Graviton savings tips before fleet creation. MCP deploy results include `estimated_cost_per_hour` and `instance_guidance` fields. The deploy help text includes a quick-reference instance type comparison. Unknown instance types are silently skipped.
- **Guided error messages**: Deploy and container commands wrap errors through `diagnose.DeployError()` / `diagnose.ContainerError()` which pattern-match error strings against known failure modes and append actionable fix suggestions. Game build errors go through `diagnoseBuildError()` in `internal/game/builder.go` which scans RunUAT logs for known patterns. Both use the same table-driven approach.
- **Progress indicators**: Long-running builds (engine compile, game server/client BuildCookRun) use `progress.Start()` to print elapsed-time messages every 2 minutes, preventing confusion during multi-hour builds with long silent linking phases.

## Configuration

Config template: `ludus.example.yaml`. User config: `ludus.yaml` (gitignored). Key settings: engine source path, engine version (optional — auto-detected from `Engine/Build/Build.version` if omitted), max compile jobs (0 = auto-detect from RAM), engine build backend (`native` or `docker`), engine Docker image (pre-built URI or local name), Docker base image (`ubuntu:22.04` default, supports apt-get and dnf bases), project name (`Lyra` default), server map (`L_Expanse`), server port (7777 UDP), deploy target (`gamelift` default, `stack`, `binary`, `anywhere`, or `ec2`), GameLift instance type (`c6i.large`), container group name, AWS region/account, EC2 fleet config (S3 bucket, SDK version), Anywhere config (location name, IP address, AWS profile).

The `game:` section supports any UE5 project:
```yaml
game:
  projectName: "MyGame"           # Required for non-Lyra projects
  projectPath: "/path/to/MyGame.uproject"  # Required for non-Lyra projects
  contentSourcePath: "/path/to/downloaded/content"  # For Lyra: Epic Launcher download path; ludus init --fix overlays into engine tree
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
- Lyra Content assets are NOT in the GitHub source repo — must be downloaded from the Epic Games Launcher Marketplace ("Lyra Starter Game") and the **entire project** must be overlaid onto the engine's `Samples/Games/Lyra/` directory. This includes both the top-level `Content/` directory AND `Plugins/GameFeatures/*/Content/` directories (ShooterCore, ShooterExplorer, ShooterMaps, ShooterTests, TopDownArena each have their own Content folder with GameFeatureData assets). Missing plugin content causes cook failures (ExitCode=25, "GameFeatureData is missing"). The Epic Games Launcher does not run on Linux; Windows or macOS required for this one-time download. Setting `game.contentSourcePath` in `ludus.yaml` and running `ludus init --fix` automates the overlay (uses `robocopy` on Windows, `cp -a` on Unix).
- RAM is critical — UE5 linking can spike 8+ GB per job; `maxJobs` controls parallelism to prevent OOM
- UE 5.6 Lyra has multiple server targets (LyraServer, LyraServerEOS, LyraServerSteam, LyraServerSteamEOS) — `DefaultServerTarget=LyraServer` must be set in DefaultEngine.ini
- UE 5.6's Gauntlet test framework directly depends on Magick.NET 14.7.0 with known CVEs; combined with TreatWarningsAsErrors, AutomationTool script modules fail to compile without `NuGetAuditLevel=critical`. Ludus sets this as an environment variable on RunUAT child processes (MSBuild reads env vars as properties), avoiding engine source modifications.
- GameLift integration uses Amazon's official Game Server Wrapper (Go binary, PID 1 in container) — no game code changes needed. The wrapper handles InitSDK, ProcessReady, and health checks; the UE5 server runs unmodified as a child process.
- Container must run as non-root user (Unreal server requirement)
- Server builds are Linux x86_64 only (matches GameLift requirement) — can be cross-compiled from Windows using Epic's cross-compile toolchain
- Client builds support Linux and Win64; native Win64 builds work if UE5 is built from source on Windows
- `ludus connect` launches the client directly on both platforms (Windows: `os/exec` child process, Linux: `syscall.Exec` process replacement). On Linux with a Win64 client, it prints copy/run instructions instead.
- UE5 content cooking requires 24+ GB RAM; 32 GB recommended. On Ubuntu, `systemd-oomd` kills the cook process at 50% memory pressure — disable it before building (`sudo systemctl disable --now systemd-oomd systemd-oomd.socket`)
- UE 5.6.1 on Windows requires specific source patches and toolchain versions — see `UE_SOURCE_PATCHES.md` for details (INITGUID fix for NNERuntimeORT on SDK >= 26100 — 5.6 only, fixed by Epic in 5.7; MSVC 14.38 toolchain requirement). Structural validation has been run against UE 5.4.4, 5.5.4, 5.6.1, and 5.7.3. INITGUID version-gating tested end-to-end on Windows (SDK 10.0.26100.0): patch correctly applied on 5.6.1 and skipped on 5.4.4, 5.5.4, and 5.7.3.

## CI / Linting

GitHub Actions CI (`.github/workflows/ci.yml`) runs on push/PR to `main`:

- **Lint** — `golangci-lint` on both Ubuntu and Windows (separate jobs to cover platform-specific build tags)
- **Build** — `go build` + `go vet` on both OSes
- **Test** — `go test` on both OSes

Lint config (`.golangci.yml`, v2 format) enables: errcheck, govet, ineffassign, staticcheck, unused, gocritic, misspell, unconvert, gosec, dupl, gofmt. Dupl uses the default threshold (150 tokens) to detect copy-paste code blocks. Gosec exclusions: G104 (unhandled errors — best-effort cleanup), G115 (integer overflow — bounded values), G204 (subprocess with variable — intentional), G301 (directory permissions 0755), G304 (file inclusion via variable — intentional), G306 (WriteFile 0644), G702 (command injection taint — same as G204), G703 (path traversal taint — same as G304). Errcheck exclusions via `std-error-handling` preset (defer Close, fmt.Fprint, os.Remove).

Run lint locally (golangci-lint v2 required — v1 does not support Go 1.24):
```bash
golangci-lint run ./...
```

Pre-commit hooks (`.hooks/pre-commit`) run `go build`, `golangci-lint` (falls back to `go vet` if not installed), and `go test` before each commit. Activate with:
```bash
git config core.hooksPath .hooks
```

## Dependencies

Go 1.24, Cobra v1.10.2 (CLI), Viper v1.21.0 (config/YAML), AWS SDK for Go v2 (GameLift, IAM, CloudFormation, S3, STS, config, credentials, SSO for auth), MCP Go SDK v1.3.1 (Model Context Protocol server).

## Cross-Platform Notes

Server builds can be cross-compiled from Windows using Epic's Linux cross-compile toolchain. `ludus init --fix` auto-downloads and installs the correct toolchain for the detected engine version (400-600 MB installer). The client build and connect commands work on both Linux and Windows.

On Windows:
1. `go build -o ludus.exe -v .`
2. Configure `ludus.yaml` with `engine.sourcePath` pointing to the Windows UE5 source (engine version is auto-detected)
3. `ludus.exe init --fix` — installs prerequisites including Linux cross-compile toolchain
4. `ludus.exe game build` — cross-compiles Linux dedicated server from Windows
5. `ludus.exe game client --platform Win64 --verbose` — builds the Win64 game client
6. `ludus.exe deploy ec2 --with-session` — deploys to GameLift Managed EC2 and creates a game session
7. `ludus.exe connect` — launches the client directly and connects to the server

Windows-specific prerequisites detected by `ludus init` (auto-fixed with `--fix` where noted):
- Visual Studio with "Desktop development with C++", "Game development with C++", and MSVC v14.38 component **(auto-fix: launches VS Installer in passive mode)**
- `BuildConfiguration.xml` at `%APPDATA%\Unreal Engine\UnrealBuildTool\` to pin MSVC version **(auto-fix)**: UE 5.4–5.6 pin `14.38.33130`; UE 5.7+ pin `14.44.35207` with `<Compiler>VisualStudio2026</Compiler>` (required for UBT to resolve VS 2026 toolchains)
- Linux cross-compile toolchain (`LINUX_MULTIARCH_ROOT`) **(auto-fix: downloads and runs installer)**: UE 5.4→v22/clang-16, 5.5→v23/clang-18, 5.6→v25/clang-18, 5.7→v26/clang-20. Installer sets the system env var; the game builder auto-detects the value from the Windows registry if the current shell hasn't picked it up yet.
- Windows SDK version detection; warns if build >= 26100 (requires NNERuntimeORT patch)
- NNERuntimeORT INITGUID patch in `Engine/Plugins/NNE/NNERuntimeORT/Source/NNERuntimeORT/NNERuntimeORT.Build.cs` **(auto-fix)**
- Plugin DLL dependency fixes **(auto-fix)**: Version-gated fixes for plugin DLLs not in the DLL search path during cook. Uses a table-driven approach (`knownPluginDLLFixes`) since Epic reorganizes plugin modules across versions — fixes must be pinned to specific versions to avoid class registration conflicts. Current fixes: UE 5.6 copies 4 Dataflow DLLs (HairStrands dependency); UE 5.7 copies 3 PlatformCrypto DLLs (AESGCMHandlerComponent dependency). See `checker_windows.go` for details.

Note: VS component detection uses individual component IDs (not workload IDs like `NativeDesktop`/`NativeGame`) for cross-VS-version compatibility — VS 2026 doesn't report workload IDs via vswhere. VS Installer `--passive` mode runs via elevated PowerShell (`Start-Process -Verb RunAs`) for UAC compliance.

## Validated End-to-End

- Linux: Engine → Lyra server → container → ECR → GameLift fleet → game sessions (UDP connectivity confirmed)
- Windows: Win64 client built → connected to GameLift fleet → played on live Linux server container
- Windows cross-version E2E (UE 5.4.4, 5.5.4, 5.6.1, 5.7.3): Full pipeline tested on each — engine build, Lyra server cross-compile (Shipping), EC2 fleet deploy, game session, Win64 client build, client connect + gameplay confirmed
- Windows INITGUID version-gating: `ludus init --fix` tested against UE 5.4.4, 5.5.4, 5.6.1, 5.7.3 (SDK 10.0.26100.0) — patch applied only on 5.6, skipped on all others
- Windows plugin DLL fixes: Dataflow DLL copy tested on 5.6.1 (cook succeeds), correctly skipped on 5.4.4, 5.5.4, and 5.7.3 (where it would cause class conflicts). PlatformCrypto DLL copy tested on 5.7.3 (resolves AESGCMHandlerComponent load failure). Table-driven approach in `checkPluginDLLDeps()` ensures version-specific fixes are only applied where needed.
- Windows engine build: `ludus engine build` tested against UE 5.4.4, 5.5.4, 5.6.1 (MSVC 14.38 + VS 2026), and 5.7.3 (MSVC 14.44 + VS 2026) — all succeeded. UE 5.7.3 `GenerateProjectFiles.bat` has a known UBT bug (hardcoded VS 2022 preference in project generation path); `Build.bat` works correctly, so GenerateProjectFiles failure is non-fatal on Windows.

## Distribution

### GoReleaser

`.goreleaser.yml` (v2 format) builds 5 binaries: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64. CGO disabled. Version injected via ldflags: `-X github.com/devrecon/ludus/internal/version.Version={{.Version}}`. Archives: `.tar.gz` (Linux/macOS), `.zip` (Windows).

### npm wrapper (`npm/`)

The `ludus-cli` npm package provides zero-install MCP configuration via `npx ludus-cli mcp`. On `npm install`, `install.js` downloads the correct pre-built binary from GitHub Releases based on `process.platform`/`process.arch`. `run.js` forwards all args and stdio to the binary (critical for MCP JSON-RPC over stdio).

MCP client configuration with npx:
```json
{
  "mcpServers": {
    "ludus": {
      "command": "npx",
      "args": ["-y", "ludus-cli", "mcp"]
    }
  }
}
```

### Release process

1. Tag a commit: `git tag v0.1.0 && git push origin v0.1.0`
2. GitHub Actions (`.github/workflows/release.yml`) triggers on `v*` tag push
3. GoReleaser builds all 5 targets and creates a GitHub Release with binaries
4. npm package version is set from the tag, then published to npmjs.org

Requires `NPM_TOKEN` secret in the GitHub repo for npm publish. `GITHUB_TOKEN` is auto-provided.

### Version

`internal/version/version.go` holds a `Version` variable (default `"dev"`) set at build time. Used by `rootCmd.Version` (enables `ludus --version`) and the MCP server implementation name.

## Roadmap

See [ROADMAP.md](ROADMAP.md) for the full prioritized roadmap. Key categories:

- **Stabilization** — ~~UE 5.4 C4756 patch~~, ~~OOM detection~~, ~~UAC failure detection~~, ~~build failure diagnostics~~ (all done in PR #35)
- **Onboarding** — ~~`ludus setup` wizard~~ (PR #43), ~~auto-detect engine version~~, ~~AWS credential validation~~ (PR #38), ~~"what's next" guidance~~ (PR #37), ~~Lyra auto-discovery~~ (PR #41), ~~server map validation~~ (PR #38)
- **Build UX** — ~~Progress indicators~~ (PR #42), resume/incremental builds, ~~build config guidance~~ (PR #39)
- **Deploy UX** — ~~Cost estimates~~ (PR #38), ~~auto-session (`--with-session`)~~ (PR #37), ~~batch destroy~~ (PR #39), ~~instance type guidance~~ (PR #41)
- **Diagnostics** — ~~`ludus doctor` command~~ (PR #42), ~~guided error messages~~ (PR #42)
- **Multi-version** — ~~`ludus config set`~~ (PR #39), ~~state profiles~~ (PR #43)
- **Code quality** — ~~`dupl` linter + refactor duplicated code~~ (PR #40)
- **Security** — Dockerfile security scanning (Hadolint, Trivy/Grype)
- **Features** — ~~ARM/Graviton support~~ (PR #36), npm package for MCP distribution, BuildGraph XML generation, studio infrastructure provisioning
