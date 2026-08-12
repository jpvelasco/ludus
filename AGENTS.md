# Ludus Development Guide

This guide provides essential context for AI agents working with Ludus, a CLI tool that automates the end-to-end pipeline for building Unreal Engine 5 dedicated servers and deploying them to AWS GameLift, GameLift Anywhere, managed EC2 fleets, CloudFormation stacks, or binary output.

See also [ARCHITECTURE.md](ARCHITECTURE.md) for the module map and design decisions, [README.md](README.md) for user-facing docs, and [UE_SOURCE_PATCHES.md](UE_SOURCE_PATCHES.md) for source-patch details. This file is the agent-facing guide.

## Build, Test & Lint

```bash
# Build (Windows)
go build -o ludus.exe -v .

# Build (Linux/macOS)
go build -o ludus -v .

# Lint (golangci-lint v2 required; CI uses golangci-lint-action v9)
golangci-lint run ./...

# Test (all packages)
go test ./...

# Test with race detector (Linux/macOS; requires CGO)
go test -race ./...

# Test (single package)
go test -v ./internal/toolchain

# Test (single test)
go test -v -run TestParseBuildVersion ./internal/toolchain

# Coverage baseline (use this exact command so numbers are comparable across runs)
go test -coverprofile=.tmp/cover.out -covermode=atomic ./... && go tool cover -func=.tmp/cover.out | tail -1

# Per-package coverage for a subset
go tool cover -func=.tmp/cover.out | rg "cmd/pipeline|cmd/engine|cmd/game|cmd/mcp"

# Static analysis
go vet ./...

# Module cleanup
go mod tidy

# Pre-commit hooks check
.hooks/pre-commit
```

Git hooks live in `.hooks/` — activate with `git config core.hooksPath .hooks`. `pre-commit` runs build + lint + tests (falls back to `go vet` when golangci-lint is blocked), `commit-msg` enforces Conventional Commits, and `pre-push` fails if a changed Go function has 0% coverage (early warning only — CI's Codecov patch gate is the real authority; `--no-verify` bypasses it locally).

## CI & Merge Gates

`.github/workflows/ci.yml` runs Lint (ubuntu + windows), Build (ubuntu/windows/macos, each also `go vet`), Test (ubuntu/windows/macos), plus `govulncheck`, gosec, Trivy filesystem scan, and a GoReleaser snapshot with a linux-binary smoke test. Separate workflows: `codeql.yml`, `octopus.yml` (automated PR review), `socket.yml`, `codacy-coverage.yml`, `release.yml`.

Dependabot is grouped (`.github/dependabot.yml`) so coupled upgrades land as one PR: `aws-sdk` (all `github.com/aws/*`, minor+patch), `codeql-action` and `artifact-actions` (both including majors — must move in lockstep), and a catch-all `actions` group. Grouping applies to version updates only, so security advisories still arrive as individual fast-trackable PRs. When a grouped Dependabot PR conflicts after another merges, comment `@dependabot rebase` rather than resolving by hand.

Only **6 checks are required**: Build (ubuntu/windows), Lint (ubuntu/windows), Test (ubuntu/windows). macOS legs, security scanners, and Codacy/Codecov are soft — they inform, they don't gate.

`main` is protected by the `protect-main` repo **ruleset**, not classic branch protection (`gh api repos/.../branches/main/protection` 404s; use `gh api repos/.../rules/branches/main`). It requires the 6 checks above, all review threads resolved (`required_review_thread_resolution: true` — including bot reviewers: Codacy, Octopus, Codex/chatgpt-codex-connector), and `strict_required_status_checks_policy: true` — a PR must be up-to-date with `main` (`gh pr update-branch` when `mergeStateStatus` is `BEHIND`). Deletion and non-fast-forward are blocked; `required_approving_review_count: 0`, so thread resolution rather than approval count is the human gate. A required check occasionally re-enters `pending` after first going green, briefly showing `BLOCKED`; re-watch and retry rather than assuming failure.

## Key Project Structure

- `main.go` → `cmd/root/root.go` → subcommand packages in `cmd/`
- `cmd/root/init.go` — the `init` command lives here, not in a separate `cmd/init/` package
- `cmd/globals/globals.go` — shared mutable state (`Cfg`, `Verbose`, `DryRun`, `JSONOutput`, `Profile`)
- `cmd/mcp/` — MCP server with 26 tools for AI orchestration
- `internal/` — all business logic (unexported); most files stay close to one primary type, but some packages are deliberately split across sibling files by concern when complexity gets too high
- Platform-specific files: `_windows.go` / `_unix.go` with `//go:build` tags
- Top-level commands (registered in `cmd/root/root.go`): `buildgraph`, `ci`, `config` (`cmd/configcmd/`, `get`/`set` only via dot-notation keys — no `view`), `connect`, `container`, `ddc`, `deploy`, `doctor`, `engine`, `game`, `logs`, `mcp`, `pipeline` (`run`), `resources`, `setup`, `status`

## Critical Features to Understand

### Deployment & Build Backends
- `--backend native` (default, builds on host)
- `--backend docker` (builds engine container images, needs Docker)
- `--backend podman` (Docker alternative, on Windows or Linux)
- `--backend wsl2` (native Linux I/O on Windows)
- macOS is supported through container backends only (`docker` or `podman`); engine images use `linux/amd64`, while `--arch arm64` still cross-compiles Graviton server binaries inside that environment
- On Windows, `--backend wsl2 --wsl-native` syncs engine sources to WSL2 ext4 for faster I/O; the plain WSL2 mode uses `/mnt/<drive>` paths

### Architecture Support
- `--arch amd64` (default)
- `--arch arm64` (Graviton instances for AWS)
- Cross-compilation from Windows to Linux binaries

### DDC (Derived Data Cache) Modes
- `--ddc zen` (default, persistent UE Zen Store cache)
- `--ddc local` (legacy FileSystem cache, deprecated — recommend `zen`)
- `--ddc none` (disabled)

### Deploy Targets
- Five targets implement `deploy.Target` (`internal/deploy/target.go`): `gamelift` (default/empty), `stack`, `ec2`, `anywhere`, `binary`. Factory switch in `cmd/globals/resolve.go`; `ResolveSessionTarget` falls back to the target recorded in state.
- The pipeline checks `target.Capabilities()` (`NeedsContainerBuild`, `SupportsSession`, etc.) to skip irrelevant stages — e.g. `anywhere` and `ec2` skip container build.

### Observability & Privacy
- Build logs are written to `.ludus/logs/` by default with retention configured under `observability.logs`
- Optional OpenTelemetry export is configured under `observability.otlp` and honors standard `OTEL_*` environment variables
- Human-readable terminal output masks AWS account IDs by default via `privacy.maskAccountId`; JSON and MCP responses are never masked, and `--show-account-id` overrides masking per run

### Coverage

- Coverage is uploaded to Codecov from **all three test legs** (ubuntu/macos/windows) via OIDC (`use_oidc: true`, `fail_ci_if_error: false`); ubuntu/macos add `-race`. Windows coverage profiles are CRLF-stripped before upload — Codecov's parser rejects CRLF and the upload silently lands in ERROR state. `fail-fast: false` so one failing leg can't cancel the others' uploads.
- Codacy gets its own coverage upload from the ubuntu leg only, via a trusted `workflow_run` (`codacy-coverage.yml`). It downloads only that run's coverage artifact and attributes it with `CODACY_COMMIT_UUID`; it never checks out or executes the untrusted revision, and only the pinned coverage action receives `CODACY_REPOSITORY_API_TOKEN`. Codacy scores only files in the uploaded profile, so Windows/darwin-only files get empty (excluded) rather than 0% coverage.
- Codacy static analysis is native (not a client-side CI upload). Revive is limited to its recommended high-severity `range` and built-in identifier checks; `golangci-lint` remains the owner of Go style, documentation, and unused-code policy.
- Codacy's OpenGrep/Semgrep analyzer has no native Windows runner. Verify it from WSL/Linux, and do not treat the native analysis CLI's `unsupported platform: win32-x64` message as success even if the process exits zero. The repo root `.semgrepignore` (and `.codacy/tools-configs/semgrep.yaml`) scope that analyzer; `.codacy.yml` excludes `.codacy/`, `dist/`, `npm/`, `scripts/`, `codacy_issues_*.json`, `coverage*.tmp` — keep those in sync when you add generated/scratch paths.
- `internal/wrapper/**` is ignored in Codecov but scored in Codacy (~28%) — a separate PID-1 binary that is E2E-covered; accepted, not papered over.
- When the Codacy MCP server is configured, run `codacy_cli_analyze` after any successful file edit (`rootPath` = workspace, `file` = edited file). After dependency changes (`go.mod`, `npm/package.json`), run it with `tool: "trivy"`. Use `provider: gh`, `organization: jpvelasco`, `repository: ludus`. If the MCP server is not available, skip silently.
- Patch coverage is enforced at 80% in `codecov.yml`; new or changed lines under that threshold post a failing `codecov/patch` status.
- It is a soft block, not a required check, so genuinely E2E-only code can still merge with judgment.

## Development Environment

- Go 1.25.12 required (see `go.mod`; CI follows it via `go-version-file`)
- Supported engine versions: UE 5.4–5.8 (`toolchainMap` in `internal/toolchain/toolchain.go`; 5.7 and 5.8 share the v26 toolchain)
- Linux or Windows with Docker/Podman for container builds
- macOS with Docker/Podman for container builds only
- AWS CLI v2 configured with credentials
- UE5 source with Lyra game assets (must be downloaded manually)
- 16+ GB RAM recommended (UE5 compilation is memory-hungry)
- 300+ GB disk space for native engine builds; container engine builds can need roughly 2 TB because UE images are very large

## Important Operational Details

- Run external processes through `runner.Runner` (not raw `exec.Command`)
- Use `internal/retry` for AWS/Docker operations (default retry strategy)
- Use `awsutil.Poll` for AWS wait loops
- Use `internal/awsenv` (NewResolver + Requirements + ImageURI/RegistryURI) for all account/region resolution and ECR URI building; centralized to address per-command duplication (see #367)
- All CLI commands support `--verbose`, `--dry-run`, JSON output
- Commands support `--profile` (creates `ludus-<name>.yaml`)
- Config loaded from `ludus.yaml` via Viper (override via `--config`)
- Build logs go to `.ludus/logs/` (project-local)
- Cache in `.ludus/cache.json` (skip unchanged stages automatically)
- State in `.ludus/state.json` (fleet IDs, ECR URIs, session data)
- Named profiles use `.ludus/profiles/<name>.json` for state isolation
- Do not hand-roll account ID masking; use the existing globals/masking helpers so terminal output stays masked while JSON/MCP remains unmasked

## Communication Model

Ludus uses the [Model Context Protocol](https://modelcontextprotocol.io/).
The MCP server is started with `ludus mcp` and exposes 26 tools for AI orchestration. Use the async `_start` tools for long native or WSL2 engine/game builds and poll with `ludus_build_status`; async container builds are not supported yet.

## Command Execution Patterns

- Commands produce output via `fmt.Printf` (status) and `fmt.Fprintln(os.Stderr)` (warnings)
- All test commands expect the working directory to be the project root
- Commands with long-running operations provide `*_start` variants for async operations
- Environment variables and CLI flags are merged with config precedence:
  `ludus.yaml` → `--flag` → `mcp-parameter`
- Architecture aliases normalize through config helpers (`amd64`/`x86_64`, `arm64`/`aarch64`); use those helpers instead of local string switches
- Container-runtime selection must distinguish engine/game build backends (`native`, `docker`, `podman`, `wsl2`) from image-build runtimes (`docker`, `podman`)

## Code Conventions

- **Errors**: `fmt.Errorf("context: %w", err)`. No sentinel errors, no custom types. AWS errors via `smithy.APIError` + `errors.As()`. Add user-facing hint patterns to `internal/diagnose/`, not command code.
- **Output**: `fmt.Printf`/`fmt.Println` for status, no logging library; JSON conditional on `globals.JSONOutput`. Human-readable stdout is filtered through `internal/output` (account-ID masking) when `privacy.maskAccountId` is on and `--show-account-id` is not set — add masking patterns to `internal/output/sanitize.go`, not at call sites.
- **Shell execution**: always through `runner.Runner`, never raw `exec.Command`; construct CLI runners via `globals.NewRunner()` so build-log teeing is wired in.
- **Platform code**: `_windows.go` / `_unix.go` / `_darwin.go` / `_linux.go` suffixes with `//go:build` tags (`internal/prereq/` is the densest example).
- **Imports**: two groups separated by a blank line — stdlib first, then third-party and project imports together (alphabetically sorted). Aliases only to resolve conflicts (e.g. `gltypes`, `cftypes`, `mcpsdk`).
- **Naming**: acronyms fully uppercase (`ID`, `URI`, `ARN`, `ECR`). Constructors `New*` returning a pointer. Single-letter pointer receivers matching the type initial (`b *Builder`). `context.Context` is the first parameter for all I/O or long-running methods.

## Lint Configuration

`.golangci.yml` v2 format: errcheck, govet, ineffassign, staticcheck, unused, gocritic, misspell, unconvert, gosec, dupl; `gofmt` as formatter; exclusion presets `std-error-handling`, `common-false-positives`. gosec exclusions: G104 (cleanup), G115 (bounded int math), G204/G702 (intentional subprocess), G301/G306 (dir/file perms), G304/G703 (config file reads) — mirrored in the CI `gosec` job. ST1005 suppressed for proper nouns in error strings (e.g. `Setup.sh`).

## Testing Constraints

- All tests use Go standard library only.
- Prefer table-driven tests with `tt` as the loop variable, and keep tests in the same package when they need unexported symbols.
- Use `t.TempDir()` for temporary directories, `t.Setenv()` for environment variable overrides, and `t.Chdir()` for working directory changes.
- Shared test infra — use it instead of rebuilding fixtures: `internal/testsupport/` (`FakeEngineTree`, `FakeProject`, `FakeTool`/`FakeTools` PATH-injected stubs, `RecordingRunner`) and `cmd/globals/testing.go` (`SetGlobals`, `SwapResolveTarget`). Dry-run works as a test seam; AWS HTTP stubbing does not — inject a narrow API interface instead.
- AWS/Docker/`wsl.exe`/subprocess-bound code (`gamelift`, `ec2fleet`, `stack`, `wrapper`, `wsl`, and most deploy logic) is E2E-covered; unit tests there should cover only the pure surface such as adapter `Name`/`Capabilities`, argument assembly, and parameter parsing.
- Keep each test function under cyclomatic complexity 8. Codacy's Lizard check counts test files and fails PRs otherwise; convert flat assertion chains to map/table loops and extract `t.Run` bodies into named helpers. Verify with `go run github.com/fzipp/gocyclo/cmd/gocyclo@latest -over 8 <file>` printing nothing.
- Codacy also tracks NLOC (50/function, 500/file) and parameter-count (8) patterns (`.codacy.yml`); keep helpers small, avoid broad fixture builders, and do not hide test files from analysis.
- Read mutex-guarded struct fields under the same lock in tests. CI runs `-race` on ubuntu and macOS, so unlocked reads of fields that a goroutine writes under a lock can fail CI even if they pass locally; a channel signal is not a happens-before edge for a lock-protected write.
- Unit test files stay co-located with source files.

## Other References

- **UE source patches**: Ludus patches UE source files at init/build time — see [UE_SOURCE_PATCHES.md](UE_SOURCE_PATCHES.md) for details and testing procedures. Related build-time fixups live in `internal/game/workarounds.go`.
- **Feature design specs**: approved designs are kept locally, not in the public repo — check local copies before implementing non-trivial features.
- **Release process**: tag-triggered via `release.yml` (GoReleaser → sha-256 checksums → npm OIDC publish). Follow the `release` skill; never improvise it — CHANGELOG lands before the tag, `npm/package.json` `version` stays at `0.0.0` (stamped at publish), and `v*` tags are immutable under the `protect-version-tags` ruleset.
