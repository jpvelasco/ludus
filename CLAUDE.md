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
ludus game [build|client]                     --skip-cook, --platform (Linux|Win64)
ludus container [build|push]       --tag/-t, --no-cache
ludus deploy [fleet|stack|session|destroy]  --target, --region, --instance-type, --fleet-name, --stack-name
ludus connect                      --address (ip:port override)
ludus status                       # checks: engine source/build, game build, client build, container image, fleet, session
ludus run                          # full pipeline (6+ stages)
  --skip-engine, --skip-game, --skip-container, --skip-deploy, --with-client
ludus mcp                          # start MCP server (stdio JSON-RPC)
ludus ci init                      # generate GitHub Actions workflow
  --output/-o, --enable-push, --enable-pr
ludus ci runner [install|status|uninstall]  # self-hosted runner management
  --dir, --labels, --name, --repo, --service, --delete
```

Global persistent flags (`cmd/root/root.go`): `--config`, `--verbose/-v`, `--json`, `--dry-run`.

Global mutable state lives in `cmd/globals/globals.go`: `Cfg`, `Verbose`, `JSONOutput`, `DryRun`.

### MCP server (`cmd/mcp/`)

`ludus mcp` starts a Model Context Protocol server over stdio (JSON-RPC). AI agents use the exposed tools to orchestrate the full pipeline. The server uses the official Go MCP SDK (`github.com/modelcontextprotocol/go-sdk`).

**Stdout protection**: MCP uses stdout for JSON-RPC transport. At startup, real stdout is saved for the MCP transport, then `os.Stdout` is redirected to `os.Stderr`. Each tool call uses `withCapture()` to capture output from internal packages.

**Tools** (13 total): `ludus_init`, `ludus_status`, `ludus_engine_setup`, `ludus_engine_build`, `ludus_game_build`, `ludus_game_client`, `ludus_container_build`, `ludus_container_push`, `ludus_deploy_fleet`, `ludus_deploy_stack`, `ludus_deploy_session`, `ludus_deploy_destroy`, `ludus_connect_info`.

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

- **`config`** — `Config` struct with typed sub-structs (`EngineConfig`, `GameConfig`, `ContainerConfig`, `DeployConfig`, `GameLiftConfig`, `AWSConfig`, `CIConfig`). `GameConfig` includes `ProjectName`, `ServerTarget`, `ClientTarget`, `GameTarget` fields with resolver methods (`ResolvedServerTarget()`, etc.) that default to `ProjectName+"Server"` etc. `AWSConfig` includes `Tags map[string]string` for configurable resource tagging (default: `ManagedBy: ludus`). `CIConfig` holds `WorkflowPath`, `RunnerDir`, and `RunnerLabels` for CI workflow generation and runner management. `Defaults()` returns sensible defaults with `ProjectName: "Lyra"`. `Load()` reads `ludus.yaml` via Viper, expands relative paths, gracefully returns defaults if file is missing. Backward compat: if `lyra:` key present but no `game:` key, migrates and prints deprecation warning to stderr.
- **`runner`** — Shell command executor. `Run()` and `RunInDir()` use `exec.CommandContext`. `RunOutput()` captures stdout as bytes instead of streaming (used by CI runner installer for `gh api` token output). Supports `Verbose` (prints `+ command` before running) and `DryRun` (prints without executing) modes. Streams stdout/stderr. `Env []string` field allows setting extra environment variables on child processes (merged on top of parent env, overriding matching keys).
- **`ci`** — `GenerateWorkflow(opts)` returns GitHub Actions YAML content using `fmt.Sprintf` (matches Dockerfile generation pattern). `WriteWorkflow(path, content)` creates parent dirs and writes the file. `RunnerInstaller` manages the self-hosted runner agent lifecycle: `Install()` (download, extract, configure, optionally install systemd service), `Status()` (check systemd/process), `Uninstall()` (deregister, optionally delete). `ParseRepoFromRemote()` extracts `owner/repo` from SSH or HTTPS git URLs.
- **`prereq`** — `Checker` with `RunAll()` returning `[]CheckResult`. Cross-platform checks: OS (linux/windows), engine source, toolchain (via `toolchain` package), game content (Lyra-specific or generic via `ContentValidationConfig`), Docker (warn-only on Windows), AWS CLI, Git, Go, disk space (100 GB), RAM (16 GB). Windows-specific checks via `platformChecks()`: Visual Studio workloads/components (via vswhere), MSVC 14.38 toolchain config (`BuildConfiguration.xml`), Windows SDK version, NNERuntimeORT INITGUID patch. `CheckResult` has `Warning bool` for non-fatal issues. `Checker.Fix bool` gates auto-remediation (`--fix` flag). Disk, memory, and platform checks use build-tagged files.
- **`toolchain`** — Engine version detection and cross-compile toolchain validation. `ParseBuildVersion()` reads `Engine/Build/Build.version` JSON. `DetectEngineVersion()` tries Build.version first, falls back to config string. `LookupToolchain()` maps engine major.minor (5.4→clang-16, 5.5/5.6→clang-18, 5.7→clang-20) to `ToolchainSpec`. `CheckToolchain()` orchestrates detection + platform-specific search: Linux scans `Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/Linux_x64/` and `LINUX_MULTIARCH_ROOT`; Windows checks `LINUX_MULTIARCH_ROOT` only. No build tags — uses `runtime.GOOS` for platform branching.
- **`engine`** — `Builder` for UE5 compilation. Linux: Setup.sh, GenerateProjectFiles.sh, make. Windows: Setup.bat, GenerateProjectFiles.bat, Build.bat. Targets: `ShaderCompileWorker` and `UnrealEditor` only (game server is built via RunUAT in the game stage). Auto-detects max jobs from RAM (8 GB per job).
- **`game`** — `Builder` for UE5 game packaging via RunUAT BuildCookRun. Cross-platform: `resolveRunUAT()` selects `cmd /c RunUAT.bat` (Windows) or `bash RunUAT.sh` (Linux). Uses relative script paths to avoid spaces-in-path issues with `cmd /c`. Path arguments are quoted (`-project="..."`) for the same reason. Pre-build fixups: `applyNuGetAuditWorkaround()` sets `NuGetAuditLevel=critical` as an env var on the runner (avoids writing `Directory.Build.props` into engine source; version-gated to 5.6/unknown), and `ensureDefaultServerTarget()` configures DefaultEngine.ini (game project config, not engine source; skips gracefully if INI structure doesn't match). `BuildClient()` supports `--platform` flag (Linux, Win64). All target names (`-servertargetname`, binary paths) are config-driven via `BuildOptions`. `EngineVersion` in `BuildOptions` enables version-specific workarounds.
- **`container`** — `Builder` for Dockerfile generation (Amazon Linux 2023, non-root user), `docker build`, and ECR push (login + tag + push). Project name and server target are parameterized in generated Dockerfile and wrapper config.
- **`deploy`** — `Target` interface abstracting deployment backends, with `Capabilities` (what the target needs/supports), `Deploy()`, `Status()`, `Destroy()` methods. Optional `SessionManager` interface for targets that support game sessions. Shared types: `DeployInput`, `DeployResult`, `DeployStatus`, `SessionInfo`. Implementations are in `gamelift`, `stack`, and `binary` packages; target resolution lives in `cmd/globals/resolve.go`.
- **`gamelift`** — `Deployer` for AWS GameLift via SDK v2. Creates container group definitions, IAM roles, fleets. Polls with 15s intervals / 30min timeout. Uses shared `tags` package for resource tagging. `Destroy()` tears down in reverse order, tolerating not-found errors. `TargetAdapter` wraps `Deployer` to implement `deploy.Target` and `deploy.SessionManager`. `CreateGameSession` returns `*GameSessionInfo` (SessionID, IPAddress, Port). `DescribeGameSession` checks session liveness.
- **`stack`** — `StackDeployer` for CloudFormation-based deployment. `Deploy()` generates a CF template (IAM role, container group definition, container fleet), calls `CreateStack`/`UpdateStack`, and polls until complete. `Destroy()` calls `DeleteStack`. `TargetAdapter` wraps `StackDeployer` to implement `deploy.Target` and `deploy.SessionManager` (reads fleet ID from stack outputs for session management). Stack naming: `ludus-<fleet-name>` by default.
- **`tags`** — Centralized AWS resource tagging. `Build(cfg)` constructs the full tag set from `cfg.AWS.Tags`, auto-derives `Project` from `cfg.Game.ProjectName`, ensures `ManagedBy: ludus`. Conversion helpers: `ToGameLiftTags()`, `ToIAMTags()`, `ToCFNTags()`, `ToTemplateTags()`. `Merge()` and `WithResourceName()` for tag composition.
- **`binary`** — `Exporter` implements `deploy.Target` for simple file export. `Deploy()` copies the server build directory to a configurable output dir via `cp -a`. `Status()` checks if the output dir exists and has files. `Destroy()` removes the output dir.
- **`state`** — Persistent state in `.ludus/state.json`. Tracks fleet (ID, stack name, status), session (ID, IP, port), client build (binary path, platform, output dir), and deploy (target name, status, detail). Read-modify-write via `Load()`/`Save()` with typed update helpers (`UpdateFleet`, `UpdateSession`, `UpdateClient`, `UpdateDeploy`, `ClearSession`, `ClearFleet`).
- **`status`** — Extracted from `cmd/status/status.go`. `StageStatus` type and check functions (`CheckEngineSource`, `CheckEngineBuild`, `CheckServerBuild`, `CheckContainerImage`, `CheckClientBuild`, `CheckDeployTarget`, `CheckGameSession`). `CheckAll(ctx, cfg, target)` runs all checks and returns `[]StageStatus`. Used by both `cmd/status` (CLI display) and `cmd/mcp` (MCP tool).

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
- **Config override**: Deploy subcommands accept `--target`, `--region`, `--instance-type`, `--fleet-name`, `--stack-name` flags that override `ludus.yaml` values.
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
- UE 5.6's Gauntlet test framework directly depends on Magick.NET 14.7.0 with known CVEs; combined with TreatWarningsAsErrors, AutomationTool script modules fail to compile without `NuGetAuditLevel=critical`. Ludus sets this as an environment variable on RunUAT child processes (MSBuild reads env vars as properties), avoiding engine source modifications.
- GameLift integration uses Amazon's official Game Server Wrapper (Go binary, PID 1 in container) — no game code changes needed. The wrapper handles InitSDK, ProcessReady, and health checks; the UE5 server runs unmodified as a child process.
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

Lint config (`.golangci.yml`, v2 format) enables: errcheck, govet, ineffassign, staticcheck, unused, gocritic, misspell, unconvert, gosec, gofmt. Gosec exclusions: G104 (unhandled errors — best-effort cleanup), G115 (integer overflow — bounded values), G204 (subprocess with variable — intentional), G301 (directory permissions 0755), G304 (file inclusion via variable — intentional), G306 (WriteFile 0644), G702 (command injection taint — same as G204), G703 (path traversal taint — same as G304). Errcheck exclusions via `std-error-handling` preset (defer Close, fmt.Fprint, os.Remove).

Run lint locally (golangci-lint v2 required — v1 does not support Go 1.24):
```bash
golangci-lint run ./...
```

Pre-commit hooks (`.hooks/pre-commit`) run `go build`, `golangci-lint` (falls back to `go vet` if not installed), and `go test` before each commit. Activate with:
```bash
git config core.hooksPath .hooks
```

## Dependencies

Go 1.24, Cobra v1.10.2 (CLI), Viper v1.21.0 (config/YAML), AWS SDK for Go v2 (GameLift, IAM, CloudFormation, config, credentials, STS/SSO for auth), MCP Go SDK v1.3.1 (Model Context Protocol server).

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

## Validated End-to-End

- Linux: Engine → Lyra server → container → ECR → GameLift fleet → game sessions (UDP connectivity confirmed)
- Windows: Win64 client built → connected to GameLift fleet → played on live Linux server container

## Roadmap

### Done

- ~~Pluggable deployment targets~~ — `deploy.Target` interface with `gamelift`, `stack`, and `binary` implementations
- ~~Cross-compile toolchain management~~ — `toolchain` package with engine version detection and clang SDK mapping
- ~~Eliminate engine source modifications~~ — Environment variables and version-gated patches instead of modifying engine source
- ~~AI agent orchestration (MCP)~~ — `ludus mcp` server with 13 tools (see Architecture > MCP server section)
- ~~GitHub Actions / CI integration~~ — `ludus ci init` generates GitHub Actions workflow files; `ludus ci runner install|status|uninstall` manages self-hosted runner agents (see Architecture > CI integration section)
- ~~CloudFormation deployment~~ — `ludus deploy stack` for atomic, declarative deployments with automatic rollback. Centralized, configurable tagging via `aws.tags` in `ludus.yaml`

### Mid-term (CI/CD and broader adoption)
- **Docker build backend** — Support building via a private engine Docker image (`ludus build --backend docker`) as an alternative to native engine builds. Studios build UE5 from source inside a Docker image once, push to a private registry (ECR, private Docker Hub), and CI jobs pull it for game builds. Epic's EULA allows this for internal use — the restriction is on public distribution of pre-built engine binaries, not private images within an organization.
- **Build caching** — Skip unchanged pipeline stages based on file hashes. Track build artifacts and skip engine/cook stages when inputs haven't changed.

### Long-term (orchestration and ecosystem)

- **BuildGraph / DAG-based orchestration** — Define build steps as a directed acyclic graph instead of a linear pipeline. Enables parallelization (e.g., server + client builds simultaneously), distributed execution across machines, artifact caching to skip unchanged steps, and pluggable VCS support (Git, Perforce, Plastic SCM). A VCS-agnostic alternative to Horde for studios that don't want the Perforce lock-in. This is where Ludus and UET would converge most — UET's core strength is dynamic BuildGraph generation. Ludus's approach would differ: deployment-aware DAGs (build + containerize + deploy as graph nodes), AI-driven graph optimization via MCP, and Git-native rather than Perforce-centric. Competing on pure BuildGraph complexity is low-ROI; the value is in extending the graph through deployment.
- **Studio infrastructure provisioning** — Potentially a separate project that provisions game studio infrastructure on AWS (Perforce, CI/CD build farms, derived data cache, virtual workstations) as composable, pluggable modules that integrate with Ludus. AWS's [cloud-game-development-toolkit](https://github.com/aws-games/cloud-game-development-toolkit) (94 stars, Terraform, MIT-0) covers this space with modules for Perforce, Horde, Jenkins, TeamCity, Cloud DDC, and VDI — but is Perforce-centric and tightly coupled to Terraform. A Ludus-ecosystem alternative could be Git-native, engine-agnostic, and composable with the Ludus pipeline (e.g., `ludus deploy horde` or a separate CLI that provisions infrastructure Ludus can target). Decision point: integrate with the existing toolkit, wrap it, or build from scratch. Parked for now — revisit once pluggable deployment targets and BuildGraph are done.
- **WSL2 support** — OS prereq check update, `.wslconfig` memory guidance, Linux filesystem for I/O performance
- **macOS support** (stretch goal) — Mac-specific engine scripts (Setup.command, Xcode), cross-compilation strategy
- **Epic Launcher content automation** — Detect `legendary` CLI on Linux as alternative to Epic Games Launcher
