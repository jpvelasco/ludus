# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

See also [AGENTS.md](AGENTS.md) for full coding guidelines, [ARCHITECTURE.md](ARCHITECTURE.md) for the module map and design decisions, and [README.md](README.md) for user-facing docs (DDC details, deployment matrix, build-time estimates, MCP client setup). Config template: `ludus.example.yaml`.

## Build / Lint / Test

```bash
go build -o ludus.exe -v .                                          # Build (Windows)
go build -o ludus -v .                                               # Build (Linux/macOS)
golangci-lint run ./...                                              # Lint (v2 required; CI uses golangci-lint-action v9)
go test ./...                                                        # All tests
go test -race ./...                                                  # All tests with race detector (Linux only)
go test -v ./internal/toolchain                                      # Single package
go test -v -run TestParseBuildVersion ./internal/toolchain           # Single test
go vet ./...                                                         # Static analysis
go mod tidy                                                          # Clean up module deps
```

Pre-commit hooks at `.hooks/pre-commit` run build + lint + tests. Activate: `git config core.hooksPath .hooks`.

CI runs 6 required checks on PRs: Build (ubuntu/windows), Lint (ubuntu/windows), Test (ubuntu/windows). Ubuntu and macOS tests run with `-race`; ubuntu also runs `-coverprofile` and uploads to Codecov. Windows tests skip race detection (needs CGO).

`main` is protected by a repo **ruleset** (not classic branch protection — `gh api repos/.../branches/main/protection` 404s; use `gh api repos/.../rules/branches/main`). It requires the 6 checks above to pass, all review threads resolved (incl. bot reviewers — Codacy, Octopus, Codex/chatgpt-codex-connector), and `strict_required_status_checks_policy: true` (a PR must be up-to-date with `main` — `gh pr update-branch` when `mergeStateStatus` is `BEHIND`). A required check occasionally re-enters `pending` after first going green, briefly showing `BLOCKED`; re-watch and retry rather than assuming failure.

### Coverage (Codecov)

Coverage is uploaded from the ubuntu test leg via **OIDC** (`use_oidc: true`, `fail_ci_if_error: false` — a Codecov/network outage must never fail the test leg). Config in `codecov.yml`: **patch coverage is enforced** (`informational: false`, target 80%) — new/changed lines under 80% post a failing `codecov/patch` status (visible red). It is intentionally a *soft* block (not in the required-checks ruleset) so a skipped upload can't deadlock a PR; self-police on the red, merge past it with judgment when new code is genuinely E2E-territory. Project/component coverage stays informational (whole-repo ~41%, much of it I/O-bound).

## Architecture

Go CLI (Cobra + Viper) that orchestrates UE5 dedicated server deployment to AWS GameLift, GameLift Anywhere, managed EC2 fleets, CloudFormation stacks, or binary output. Module: `github.com/jpvelasco/ludus`, Go 1.25.12 (see `go.mod`; CI follows it).

### Pipeline

Six stages, independently runnable or chained via `ludus run`:
init → engine build → game build → container build → deploy → connect

Not all stages run for every target — the pipeline checks `target.Capabilities()` and skips irrelevant stages (e.g. `anywhere` and `ec2` skip container build).

### Code Organization

- `main.go` → `cmd/root/root.go` → subcommand packages in `cmd/`
- `cmd/globals/globals.go` — shared mutable state (`Cfg`, `Verbose`, `DryRun`, `JSONOutput`, `Profile`)
- `cmd/root/init.go` — the `init` command lives here, not in a separate `cmd/init/` package
- `internal/` — all business logic, one primary type per file, unexported packages

Beyond the six pipeline stages, these top-level commands exist:
- `ludus setup` (`cmd/setup/`) — interactive first-time wizard: scans system, auto-detects UE source/version, picks deploy target, writes `ludus.yaml` (`--profile` writes `ludus-<name>.yaml`).
- `ludus config` (`cmd/configcmd/`) — `get`/`set`/`view` for `ludus.yaml` via dot-notation keys (e.g. `ludus config set engine.version 5.7.3`).
- `ludus doctor` (`cmd/doctor/`) — deeper diagnostics than `init` (stale DLLs, toolchain mismatch, disk, partial build state, AWS credential expiry, cache integrity). Use when `init` passes but something is broken.
- `ludus ci` (`cmd/ci/`, `internal/ci/`) — `ci init` generates a GitHub Actions workflow; `ci runner` manages a self-hosted runner agent.
- `ludus logs` (`cmd/logs/`), `ludus resources` (`cmd/resources/`), `ludus ddc` (`cmd/ddc/`), `ludus buildgraph` (`cmd/buildgraph/`) — see their respective sections below.

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

`internal/ddc/` manages UE5's Derived Data Cache — persistent shader/asset cache that survives Docker container lifecycles. Three modes: `zen` (default since v0.7.0 — the Unreal Zen Store, UE's default local backend since 5.4, applies to UE 5.4+), `local` (legacy FileSystem cache, **deprecated** — delete-only in UE since 5.4; `doctor`/`ddc status`/config load warn and recommend `zen`), and `none` (disabled). The package is split by concern: `ddc.go` (mode constants, `ValidateDDCMode` which maps `""` → `zen`, env/Zen-path constants), `paths.go` (path resolution), `size.go` (dir sizing), `ops.go` (clean/prune).

Integration points live in `internal/dockerbuild/game_ddc.go` (mount-arg assembly) and `game_script.go` (build-script preamble): for `zen`, the host Zen directory (`ddc.zenPath`, default `~/.ludus/zen`) is bind-mounted to the container's fixed ZenStore data path (`ddc.ZenContainerPath` = `/home/ue/.config/Epic/UnrealEngine/Common/Zen/Data`); the preamble recursively chowns `/home/ue/.config` so `zenserver` (running as `ue`) can write it. For legacy `local`, the host DDC dir is mounted at `/ddc` and passed via `UE-LocalDataCachePath`.

`ludus ddc` subcommands: `status`, `clean`, `prune`, `warmup`. Config in `ludus.yaml` under `ddc.mode` / `ddc.zenPath` / `ddc.localPath`, overridable via `--ddc` flag.

### Build Observability

`internal/buildlog/` tees build output to per-run log files under `.ludus/logs/` (project-local). All CLI runners are constructed via `globals.NewRunner()`, which lazily opens the run log on first use and tees stdout/stderr to it; MCP async builds get a per-build-id log via `syncBuffer.sink`. `ludus logs` subcommands: `list`, `path`, `tail`. Config under `observability.logs` (`enabled`/`dir`/`retainRuns`); `--no-logs` disables. Dry-run is never logged.

`internal/tracing/` adds optional OpenTelemetry (OTLP) trace export — one span per pipeline stage under a `ludus.run` root span, wired in `cmd/pipeline/executeStages`. No-op with zero overhead unless `observability.otlp.enabled` (or standard `OTEL_*` env vars) is set. Initialized in root `PersistentPreRunE`, flushed in `Execute()`.

`internal/dflint/` lints Dockerfiles (hadolint) and scans the built image for vulnerabilities (trivy), surfaced via `ludus status`. Both tools degrade gracefully when not installed. Built-in rules live in `checks.go`; `lint.go` holds the result types and orchestration; `hadolint.go` / `trivy.go` wrap the external tools.

Several packages keep one concern per file rather than one giant file (Codacy's Lizard flags high per-file complexity). `internal/dockerbuild/game.go` is split into `game_resolver.go` (name/arch resolution), `game_script.go` (build-script generation), `game_ddc.go` (DDC mount args), and `game_container.go` (container execution); `game.go` keeps the option/builder types and the `Build`/`BuildClient` entry points. When a file's summed function complexity approaches 50, split along a clean seam into sibling files rather than refactoring logic.

### BuildGraph

`cmd/buildgraph/` generates UE5 BuildGraph XML describing engine and game build stages as a DAG. Used with Horde, UET, or other external orchestrators. Exposed as `ludus_buildgraph` MCP tool.

### MCP Server

`cmd/mcp/` exposes 26 tools via JSON-RPC over stdio. Registration in `cmd/mcp/register.go` delegates to domain-specific `register*Tools()` functions. Stdout redirected to stderr (MCP protocol uses stdout). Long-running native/WSL2 engine and game builds have async `_start` variants returning build IDs, polled via `ludus_build_status`; container builds have no async variant yet.

### GameLift Wrapper

`internal/wrapper/` is a separate Go binary (not the CLI) that gets compiled into the container image. It acts as PID 1 inside GameLift containers, forwarding signals and managing the UE5 server process lifecycle. The container build step compiles it cross-platform with `GOOS=linux`.

### Configuration Flow

`ludus.yaml` → Viper → `config.Config` struct (loaded in `PersistentPreRunE`, stored in `globals.Cfg`) → CLI flags override → MCP params override → `internal/` logic consumes.

### WSL2 Build Backend

`--backend wsl2` runs engine/game builds inside a WSL2 Linux distro without Docker. Two sub-modes controlled by `--wsl-native`:
- Default (virtiofs): builds run against the Windows filesystem mounted at `/mnt/`. Slower I/O but no sync step.
- `--wsl-native`: syncs the engine/project source to the WSL2 ext4 filesystem before building. Much faster compile times but requires disk space for the copy.

`internal/wsl/` handles distro detection, path translation, and the source sync. Use `--wsl-distro` to override the auto-detected distro.

### AWS Environment Resolution

`internal/awsenv` centralizes AWS account-ID and region resolution and ECR URI construction. Use `awsenv.NewResolver(DryRun).Resolve(ctx, cfg, awsenv.Requirements{...})` to resolve account/region (auto-detects via STS/IMDS when not configured, respects dry-run). Use `awsenv.ImageURI(env, repo, tag)`, `awsenv.RepositoryURI(env, repo)`, or `awsenv.RegistryURI(env)` for ECR URIs — these validate that account ID and region are present. Never construct `dkr.ecr` strings directly in command code.

Thin wrappers in `cmd/globals/aws.go` (`ResolveAWSAccountID`, `ResolveAWSRegion`) delegate to `awsenv` for backward compatibility; prefer calling `awsenv` directly for new code.

### AWS Polling

`internal/awsutil/poll.go` provides a generic `Poll()` helper used across deployers for waiting on fleet activation, stack events, etc. Prefer it over hand-rolled polling loops when adding new AWS wait conditions.

`awsutil.IsNotFound()` and `awsutil.IsConflict()` classify common AWS error patterns — use these in cleanup and idempotent create paths instead of inspecting error strings directly.

### State and Caching

`.ludus/state.json` — fleet IDs, session IPs, ECR URIs, build paths. Typed update helpers in `internal/state/state.go`.
`.ludus/cache.json` — input hashes per stage. Unchanged stages auto-skip. `--no-cache` forces rebuild.
Profiles (`--profile <name>`) isolate both: config from `ludus-<name>.yaml`, state in `.ludus/profiles/<name>.json`.

### Cross-Architecture Support

The `--arch` flag threads through the entire pipeline: game build → container build → deploy. Architecture mismatches are caught automatically (e.g. arm64 build with x86 instance type auto-switches to Graviton). See `internal/config/` for `NormalizeArch`, `ServerPlatformDir`, `BinariesPlatformDir`. Arch aliases (`amd64`/`x86_64`, `arm64`/`aarch64`) normalize through these helpers — don't hand-roll the string comparison.

macOS is supported only through container backends (`docker`/`podman`); there is no native engine build path on macOS. Engine container images are forced to `linux/amd64` under QEMU (Epic ships only an x86_64 Linux toolchain), even when the host is Apple Silicon. `--arch arm64` game builds still cross-compile Graviton server binaries inside that emulated amd64 environment — the engine image itself stays amd64.

## Code Conventions

Full style guide in [AGENTS.md](AGENTS.md). Key points for quick reference:

- **Errors**: `fmt.Errorf("context: %w", err)`. No sentinel errors, no custom types. AWS errors via `smithy.APIError` + `errors.As()`. `internal/diagnose/` matches error patterns to user-facing hints — add new patterns there rather than embedding hint strings in command code.
- **Output**: `fmt.Println`/`fmt.Printf` for status. No logging library. JSON conditional on `globals.JSONOutput`. Human-readable stdout is filtered through `internal/output` (account-ID masking) when `privacy.maskAccountId` is on and `--show-account-id` is not set — installed once in root `PersistentPreRunE`, skipped for `--json` and `mcp`. Mask new sensitive identifiers by adding a pattern to `internal/output/sanitize.go`, not at call sites.
- **Shell execution**: Always through `runner.Runner`, never raw `exec.Command`.
- **Tests**: stdlib only, table-driven with `tt` loop var, same-package (access unexported), `t.TempDir()` for temp dirs, `t.Setenv()` for env overrides, `t.Chdir()` for cwd-dependent tests. AWS/Docker/`wsl.exe`/subprocess-bound code (gamelift, ec2fleet, stack, wrapper, wsl, most deploy logic) relies on E2E coverage — unit tests there cover only the pure surface (adapters' `Name`/`Capabilities`, arg assembly, param parsing). Two hard constraints: (1) keep each test function under **cyclomatic complexity 8** or Codacy's Lizard check fails the PR — convert flat assertion chains to map/table loops and extract `t.Run` bodies into named helpers (`go run github.com/fzipp/gocyclo/cmd/gocyclo@latest -over 8 <file>` must print nothing); Lizard also tracks NLOC and parameter count, so keep helpers small and avoid broad fixture builders rather than hiding test files from analysis; (2) read mutex-guarded struct fields **under the same lock** in tests — CI runs `-race` on ubuntu/macos (not Windows, which lacks CGO), and a channel signal alone is not a happens-before edge with a lock-protected write.
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

Approved feature designs are kept locally (not in the public repo). Check local copies before implementing non-trivial features.

## Release Process

Releases are **tag-triggered**: pushing a `vX.Y.Z` tag to main runs `.github/workflows/release.yml` → GoReleaser builds 5 binaries → `scripts/embed-checksums.js` writes SHA-256 into `npm/package.json` → `npm publish` from `npm/`. npm auth is **OIDC trusted publishing** (`id-token: write`), so there is no npm token to manage. `scripts/validate_ue_versions.sh` validates UE version consistency at init/CI time.

Releasing is a gated, ordered process — do not improvise it:
- **CHANGELOG before tag.** Land the `vX.Y.Z` CHANGELOG entry (and any version-source bumps) on main via a normal PR with green CI *before* tagging. The tag is the last step, never the first.
- **Leave `npm/package.json` `version` at its `0.0.0` placeholder.** The workflow stamps the real version at publish time; committing a version causes a "Version not changed" failure.
- **`v*` tags are immutable** (protected against deletion/force-update by a repo ruleset). Re-tagging after a failed release is a deliberate recovery — temporarily relax the tag protection, fix on main via PR, re-tag, then restore protection — not a force-push. Never reuse a version that may already have published to npm; cut the next patch instead.

npm package: `ludus-cli`. README in `npm/README.md`, keywords in `npm/package.json`.

The published tarball is a thin shim and ships **no binary** (`bin/` is git/npm-ignored). `npm/install.js` exports `ensureBinary()`, which downloads the platform archive from the GitHub release, SHA-256-verifies it against `package.json`'s `binaryChecksums`, extracts atomically (unique temp dir + rename), and writes a `bin/.installed-version` marker. `postinstall` runs it once; `npm/run.js` (the `bin` entry) also calls it before every spawn, so a skipped postinstall (`ignore-scripts`/pnpm), a failed download, or a version-skewed binary self-heals on first run. The marker comparison short-circuits the common path (cheap file reads, no network). **All `ensureBinary` progress must go to stderr** — `ludus mcp` (JSON-RPC) and `--json` write to stdout. `LUDUS_SKIP_AUTO_DOWNLOAD=1` bypasses the self-heal for air-gapped/self-managed binaries. Tests: `cd npm && npm test` (`node --test`, network-free).

