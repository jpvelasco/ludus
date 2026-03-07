# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ludus is a Go CLI tool that automates the end-to-end pipeline for deploying Unreal Engine 5 dedicated servers to AWS GameLift. It orchestrates: UE5 source builds → game server compilation → deployment (via Docker containers, Managed EC2, or local Anywhere). Server builds can be cross-compiled from Windows for Linux x86_64 or arm64 (Graviton). While Lyra (Epic's sample game) is the default project, Ludus supports any UE5 game with dedicated server targets.

## Build & Run Commands

```bash
go build -o ludus.exe -v .        # Build (Windows)
go build -o ludus -v              # Build (Linux/macOS)
go mod tidy                       # Clean up module dependencies
go vet ./...                      # Static analysis
golangci-lint run ./...           # Lint (v2 required, must pass before commit)
go test ./...                     # Run all tests
go test -v ./internal/toolchain   # Run tests for a single package
```

## Architecture

### Entry point

`main.go` → `root.Execute()` → Cobra command dispatch. `PersistentPreRunE` loads config into `globals.Cfg`, auto-detects engine version from `Engine/Build/Build.version`.

### Command layer (`cmd/`)

Each subcommand lives in its own package under `cmd/` and exports a `Cmd *cobra.Command`. All registered in `cmd/root/root.go`.

```
ludus setup                        # interactive wizard
ludus init [--fix]                 # validate/fix prerequisites
ludus config [set|get]             # dot-notation config access
ludus engine [build|setup|push]    # --jobs, --backend, --no-cache
ludus game [build|client]          # --arch, --skip-cook, --platform, --backend, --build-config, --jobs
ludus container [build|push]       # --tag, --no-cache, --arch
ludus deploy [fleet|stack|anywhere|ec2|session|destroy]  # --target, --region, --instance-type, --with-session
ludus connect                      # launch client
ludus doctor                       # deep diagnostics
ludus status                       # pipeline stage checks
ludus run                          # full pipeline
ludus mcp                          # MCP server (stdio JSON-RPC)
ludus ci [init|runner]             # GitHub Actions workflow/runner
ludus buildgraph                   # generate BuildGraph XML for Horde/UET
```

Global flags: `--config`, `--verbose/-v`, `--json`, `--dry-run`, `--profile`. Global state: `cmd/globals/globals.go`.

### MCP server (`cmd/mcp/`)

21 tools for AI agents to orchestrate the pipeline. Stdout redirected to stderr (MCP uses stdout for JSON-RPC). `withCapture()` captures output per tool call. Async `_start` variants for long builds (`buildmgr.go`). Error convention: operational errors → `CallToolResult{IsError: true}` with JSON; Go errors → protocol failures only.

Files: `mcp.go`, `register.go`, `capture.go`, `helpers.go`, `buildmgr.go`, `tools_*.go` (one per domain).

### Implementation layer (`internal/`)

Most packages are named for what they do (`engine`, `game`, `cache`, `state`, `tags`, `ci`, etc.). Key non-obvious ones:

| Package | Why it's non-obvious |
|---------|---------------------|
| `config` | Arch helpers (`NormalizeArch`, `ServerPlatformDir`, `BinariesPlatformDir`) live here, not just config loading |
| `prereq` | Platform-specific via build tags — `checker_windows.go`/`checker_unix.go` |
| `toolchain` | Cross-compile toolchain mapping: 5.4→v22, 5.5→v23, 5.6→v25, 5.7→v26 |
| `container` | Handles `--platform`, `--provenance=false`, QEMU detection, binary name resolution |
| `deploy` | `Target` interface — factory in `cmd/globals/resolve.go`, not here |
| `wrapper` | GameLift Game Server Wrapper (Go binary, PID 1 in container) — clone, build, cache per arch |
| `dockerbuild` | Builds engine/game *inside* Docker — separate from `container` which builds *the container image* |

### Key patterns

- **Builder/Deployer types**: `New*(opts)` constructor, operation methods, structured results. All accept `context.Context`.
- **Runner abstraction**: Never call `exec.Command` directly — use `runner.Runner`.
- **Pluggable deploy targets**: `deploy.Target` interface, factory in `cmd/globals/resolve.go`. Pipeline checks `target.Capabilities()` to skip unneeded stages.
- **Config override**: CLI flags override `ludus.yaml` values. MCP tools apply overrides to config before calling shared logic.
- **Arch-aware instance auto-default**: All fleet resolvers check if instance type matches server arch via `pricing.InstanceArch()`/`pricing.DefaultInstanceType()`. Mismatched arch auto-switches (arm64→c7g.large, amd64→c6i.large).
- **State persistence**: Deploy/build commands write to `.ludus/state.json` so downstream commands can resolve fleet/session/client info.
- **Profiles**: `--profile <name>` isolates config (`ludus-<profile>.yaml`) and state (`.ludus/profiles/<name>.json`).

### Platform-specific code

Build-tagged files (`//go:build`): `prereq/checker_windows.go`/`checker_unix.go` (disk, memory, platform checks), `anywhere/process_unix.go`/`process_windows.go` (process management), `connect/launch_windows.go`/`launch_unix.go` (client launch). When adding methods in build-tagged files, a stub MUST exist in the counterpart file.

## Configuration

Config template: `ludus.example.yaml`. User config: `ludus.yaml` (gitignored).

```yaml
game:
  projectName: "MyGame"           # defaults to "Lyra"
  projectPath: "/path/to/MyGame.uproject"
  arch: "amd64"                   # or "arm64" for Graviton
  serverTarget: "MyGameServer"    # defaults to <projectName>Server
  serverMap: "MyDefaultMap"
```

Backward compat: `lyra:` key auto-migrates to `game:` with deprecation warning.

## Key Domain Context

- UE5 must be built from source (launcher builds can't produce dedicated server targets)
- Lyra content is NOT in the GitHub source — download from Epic Launcher, overlay onto `Samples/Games/Lyra/` (includes `Plugins/GameFeatures/*/Content/` dirs). `ludus init --fix` automates this.
- RAM is critical — linking spikes 8+ GB per job; `maxJobs` controls parallelism
- GameLift uses Amazon's Game Server Wrapper (Go binary, PID 1 in container) — no game code changes needed
- Container must run as non-root user (Unreal server requirement)
- Docker BuildKit provenance attestation (`--provenance`) creates OCI manifest indexes that GameLift cannot parse — ludus disables it with `--provenance=false`
- Cross-architecture container builds (arm64 on amd64 host) require QEMU emulation: `docker run --rm --privileged tonistiigi/binfmt --install arm64`
- Shipping builds produce binaries named `<Target>-<Platform>-<Config>` (e.g. `LyraServer-LinuxArm64-Shipping`), not the bare target name — container builder auto-detects via `resolveServerBinaryName()`
- UE 5.6 needs `DefaultServerTarget=LyraServer` in DefaultEngine.ini and `NuGetAuditLevel=critical` env var
- Windows cross-compile toolchains: 5.4→v22/clang-16, 5.5→v23/clang-18, 5.6→v25/clang-18, 5.7→v26/clang-20. `ludus init --fix` auto-downloads.
- Windows-specific auto-fixes: VS components, `BuildConfiguration.xml`, NNERuntimeORT INITGUID patch (5.6 only), plugin DLL fixes (version-pinned, see `checker_windows.go`)

## CI / Linting

GitHub Actions CI runs on push/PR to `main`: lint + build + test on both Ubuntu and Windows.

Lint config: `.golangci.yml` (v2 format). Key gosec exclusions: G104, G115, G204, G301, G304, G306, G702, G703.

Pre-commit hooks: `.hooks/pre-commit`. Activate: `git config core.hooksPath .hooks`

## Dependencies

Go 1.24, Cobra, Viper, AWS SDK for Go v2, MCP Go SDK v1.3.1.

## Distribution

GoReleaser (`.goreleaser.yml`) builds 5 targets (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64). npm wrapper (`npm/ludus-cli`) provides `npx ludus-cli mcp` for zero-install MCP. Release: tag → GitHub Actions → GoReleaser + npm publish.

## Roadmap

See [ROADMAP.md](ROADMAP.md).
