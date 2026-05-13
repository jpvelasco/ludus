# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

See also [AGENTS.md](AGENTS.md) for full coding guidelines, [ARCHITECTURE.md](ARCHITECTURE.md) for the module map and design decisions, and [README.md](README.md) for user-facing docs (DDC details, deployment matrix, build-time estimates, MCP client setup). Config template: `ludus.example.yaml`.

## Build / Lint / Test

```bash
go build -o ludus.exe -v .                                          # Build (Windows)
go build -o ludus -v .                                               # Build (Linux/macOS)
golangci-lint run ./...                                              # Lint (v2 required)
go test ./...                                                        # All tests
go test -race ./...                                                  # All tests with race detector (Linux only)
go test -v ./internal/toolchain                                      # Single package
go test -v -run TestParseBuildVersion ./internal/toolchain           # Single test
go vet ./...                                                         # Static analysis
go mod tidy                                                          # Clean up module deps
```

Pre-commit hooks at `.hooks/pre-commit` run build + lint + tests. Activate: `git config core.hooksPath .hooks`.

CI runs 6 required checks on PRs: Build (ubuntu/windows), Lint (ubuntu/windows), Test (ubuntu/windows). Ubuntu tests run with `-race` and `-coverprofile`; coverage summary prints via `go tool cover` in CI logs. Windows tests skip race detection (needs CGO).

## Architecture

Go CLI (Cobra + Viper) that orchestrates UE5 dedicated server deployment to AWS GameLift. Module: `github.com/jpvelasco/ludus`, Go 1.25.

### Pipeline

Six stages, independently runnable or chained via `ludus run`:
init → engine build → game build → container build → deploy → connect

Not all stages run for every target — the pipeline checks `target.Capabilities()` and skips irrelevant stages (e.g. `anywhere` and `ec2` skip container build).

### Code Organization

- `main.go` → `cmd/root/root.go` → subcommand packages in `cmd/`
- `cmd/globals/globals.go` — shared mutable state (`Cfg`, `Verbose`, `DryRun`, `JSONOutput`, `Profile`)
- `cmd/root/init.go` — the `init` command lives here, not in a separate `cmd/init/` package
- `internal/` — all business logic, one primary type per file, unexported packages

### Deploy Target System

All backends implement `deploy.Target` interface (`internal/deploy/target.go`). Five targets: `gamelift`, `stack`, `ec2`, `anywhere`, `binary`. Factory in `cmd/globals/resolve.go` instantiates the correct target from config.

To add a new deploy target:
1. `internal/<pkg>/deployer.go` — implement the deployer
2. `internal/<pkg>/adapter.go` — adapt to `deploy.Target` interface
3. `cmd/globals/resolve.go` — wire into the factory switch
4. `cmd/deploy/deploy.go` — add subcommand
5. `cmd/mcp/tools_deploy.go` — expose via MCP
6. `internal/status/status.go` — add status check

`cmd/resources/` lists all ludus-managed AWS resources, discovered by `ManagedBy=ludus` tag and known naming patterns (ECR repos, S3 build buckets) via `internal/inventory/`.

### Runner Abstraction

All shell execution goes through `runner.Runner` (`internal/runner/runner.go`), never raw `exec.Command`. Handles `--verbose` output (`+ cmd args`), `--dry-run` (print without executing), and consistent error wrapping.

Network-facing operations (Docker, AWS) wrap calls with `internal/retry/`: exponential backoff with jitter, configurable via a `Config` struct. Use `retry.Default()` for standard CLI retry behavior; pass a custom `Config` when you need different attempt counts or delays.

### DDC (Derived Data Cache)

`internal/ddc/` manages UE5's Derived Data Cache — persistent shader/asset cache that survives Docker container lifecycles. Two modes: `local` (default, persists to `~/.ludus/ddc`) and `none` (disabled).

Integration points: `internal/dockerbuild/` mounts the host DDC directory as a Docker volume and passes `UE-LocalDataCachePath=<path>` as an env override to redirect UE5's local DDC backend (no ini patching needed — `BaseEngine.ini` already configures `EnvPathOverride=UE-LocalDataCachePath`).

`ludus ddc` subcommands: `status`, `clean`, `prune`, `warmup`. Config in `ludus.yaml` under `ddc.mode` / `ddc.localPath`, overridable via `--ddc` flag.

### BuildGraph

`cmd/buildgraph/` generates UE5 BuildGraph XML describing engine and game build stages as a DAG. Used with Horde, UET, or other external orchestrators. Exposed as `ludus_buildgraph` MCP tool.

### MCP Server

`cmd/mcp/` exposes 26 tools via JSON-RPC over stdio. Registration in `cmd/mcp/register.go` delegates to domain-specific `register*Tools()` functions. Stdout redirected to stderr (MCP protocol uses stdout). Long-running builds have async variants returning build IDs.

### GameLift Wrapper

`internal/wrapper/` is a separate Go binary (not the CLI) that gets compiled into the container image. It acts as PID 1 inside GameLift containers, forwarding signals and managing the UE5 server process lifecycle. The container build step compiles it cross-platform with `GOOS=linux`.

### Configuration Flow

`ludus.yaml` → Viper → `config.Config` struct (loaded in `PersistentPreRunE`, stored in `globals.Cfg`) → CLI flags override → MCP params override → `internal/` logic consumes.

### WSL2 Build Backend

`--backend wsl2` runs engine/game builds inside a WSL2 Linux distro without Docker. Two sub-modes controlled by `--wsl-native`:
- Default (virtiofs): builds run against the Windows filesystem mounted at `/mnt/`. Slower I/O but no sync step.
- `--wsl-native`: syncs the engine/project source to the WSL2 ext4 filesystem before building. Much faster compile times but requires disk space for the copy.

`internal/wsl/` handles distro detection, path translation, and the source sync. Use `--wsl-distro` to override the auto-detected distro.

### AWS Polling

`internal/awsutil/poll.go` provides a generic `Poll()` helper used across deployers for waiting on fleet activation, stack events, etc. Prefer it over hand-rolled polling loops when adding new AWS wait conditions.

`awsutil.IsNotFound()` and `awsutil.IsConflict()` classify common AWS error patterns — use these in cleanup and idempotent create paths instead of inspecting error strings directly.

### State and Caching

`.ludus/state.json` — fleet IDs, session IPs, ECR URIs, build paths. Typed update helpers in `internal/state/state.go`.
`.ludus/cache.json` — input hashes per stage. Unchanged stages auto-skip. `--no-cache` forces rebuild.
Profiles (`--profile <name>`) isolate both: config from `ludus-<name>.yaml`, state in `.ludus/profiles/<name>.json`.

### Cross-Architecture Support

The `--arch` flag threads through the entire pipeline: game build → container build → deploy. Architecture mismatches are caught automatically (e.g. arm64 build with x86 instance type auto-switches to Graviton). See `internal/config/` for `NormalizeArch`, `ServerPlatformDir`, `BinariesPlatformDir`.

## Code Conventions

Full style guide in [AGENTS.md](AGENTS.md). Key points for quick reference:

- **Errors**: `fmt.Errorf("context: %w", err)`. No sentinel errors, no custom types. AWS errors via `smithy.APIError` + `errors.As()`. `internal/diagnose/` matches error patterns to user-facing hints — add new patterns there rather than embedding hint strings in command code.
- **Output**: `fmt.Println`/`fmt.Printf` for status. No logging library. JSON conditional on `globals.JSONOutput`.
- **Shell execution**: Always through `runner.Runner`, never raw `exec.Command`.
- **Tests**: stdlib only, table-driven with `tt` loop var, same-package (access unexported), `t.TempDir()` for temp dirs, `t.Setenv()` for env overrides, `t.Chdir()` for cwd-dependent tests. 30/34 internal packages have tests. AWS-heavy packages (ec2fleet) and interface-only packages (deploy, version) rely on E2E or integration coverage.
- **Platform code**: `_windows.go` / `_unix.go` suffixes with `//go:build` tags.
- **Imports**: Two groups separated by a blank line — stdlib first, then third-party and project imports together (alphabetically sorted). Aliases only to resolve conflicts (e.g. `gltypes`, `cftypes`).
- **Naming**: Acronyms fully uppercase (`ID`, `URI`, `ARN`, `ECR`). Constructors `New*` returning a pointer. Single-letter pointer receivers matching type initial (`b *Builder`, `d *Deployer`). `context.Context` is the first parameter for all I/O or long-running methods.

## Lint Configuration

`.golangci.yml` v2 format. Enabled: errcheck, govet, ineffassign, staticcheck, unused, gocritic, misspell, unconvert, gosec, dupl.

gosec exclusions: G104 (cleanup), G115 (bounded int math), G204/G702 (intentional subprocess), G301/G306 (dir/file perms), G304/G703 (config file reads). ST1005 suppressed for proper nouns in error strings (e.g. `Setup.sh`).

## UE Source Patches

Ludus patches UE source files at init/build time. See [UE_SOURCE_PATCHES.md](UE_SOURCE_PATCHES.md) for details and testing procedures.

## Codacy Integration

When the Codacy MCP server is configured: after any successful file edit, run `codacy_cli_analyze` with `rootPath` = workspace path and `file` = edited file (tool unset). After dependency changes (`go.mod`, `npm/package.json`), run it with `tool: "trivy"`. Use `provider: gh`, `organization: jpvelasco`, `repository: ludus` for Codacy tool calls. If Codacy MCP is not available, skip silently. Codacy config: `.codacy/codacy.yaml`.

## Feature Design Specs

Approved feature designs live in `docs/superpowers/specs/`. Check there before implementing non-trivial features — specs contain behavioral contracts, testing strategies, and decisions already made.

## Release Process

Tag `vX.Y.Z` on main → `.github/workflows/release.yml` → GoReleaser builds 5 binaries → `scripts/embed-checksums.js` writes SHA-256 into `npm/package.json` → `npm publish` from `npm/` directory. `scripts/validate_ue_versions.sh` validates UE version consistency at init/CI time.

npm package: `ludus-cli`. README in `npm/README.md`, keywords in `npm/package.json`.
