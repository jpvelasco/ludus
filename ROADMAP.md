# Ludus Roadmap

Prioritized list of planned work, organized by category. Items are roughly ordered by priority within each section.

## Stabilization

Bugs and rough edges discovered during cross-version E2E testing (UE 5.4–5.7 on Windows).

- [x] **UE 5.4 C4756 overflow patch** — `ludus init --fix` auto-patches UE 5.4 source files where NAN/INFINITY macros trigger C4756. Version-gated to 5.4 + SDK >= 26100. *(PR #35)*
- [x] **OOM detection / maxJobs halving for cross-compile** — `-j/--jobs` flag on `game build` and `game client`. Auto-detects RAM and halves parallelism for cross-compile (16GB/job vs 8GB/job). *(PR #35)*
- [x] **UAC failure detection** — Detects exit code `0xC0E90002` and provides 3 actionable remediation steps. *(PR #35)*
- [x] **Build failure diagnostics** — Scans RunUAT Log.txt for 7 known error patterns (missing content, OOM, DLL failures, NuGet, C4756, toolchain) with actionable hints. *(PR #35)*
- [ ] **Lyra project path auto-resolution in config** — `game.projectPath` is auto-detected at build time in `game.Builder.LocateProject()` but not at config load time. Commands like `buildgraph` that read config directly see an empty path. Add a `ResolvedProjectPath()` helper or resolve at `config.Load()` time when `ProjectName` is "Lyra".

## Onboarding / First-Run UX

Reducing friction for new users going from zero to a running game session.

- [x] **`ludus setup` interactive wizard** — Guided first-run setup that: scans for engine source directories (common paths + drive roots on Windows), auto-reads `Build.version` for the engine version, discovers Lyra Launcher downloads, auto-detects AWS account ID via `sts get-caller-identity`, prompts for deploy target and instance type, and writes a complete `ludus.yaml`. Profile-aware: `ludus --profile ue57 setup` writes `ludus-ue57.yaml`.
- [x] **Auto-detect engine version** — Drop the `engine.version` config requirement. `toolchain.ParseBuildVersion()` already reads `Engine/Build/Build.version` JSON from every engine source tree. If version is empty in config, read it automatically.
- [x] **AWS credential validation** — `ludus init` checks `aws sts get-caller-identity` and warns if credentials aren't configured or expired. Warning-only so engine/game builds aren't blocked.
- [x] **"What's next" guidance** — After each command succeeds, print the next step in the pipeline. After `init`: "Run `ludus engine build`". After engine build: "Run `ludus game build`". After deploy: "Run `ludus deploy session`". Etc.
- [x] **Lyra content auto-discovery** — `ludus init` scans common paths (`Documents/Unreal Projects/LyraStarterGame*`, OneDrive variants) for downloaded Lyra content. If found, suggests setting `game.contentSourcePath` or auto-overlays with `--fix`.
- [x] **Server map validation** — `ludus init` searches for `<serverMap>.umap` in the project's Content/ directory and warns if not found. Warning-only since maps can be path references or generated at cook time.

## Build UX

Improving the experience during long build operations.

- [x] **Progress indicators** — Elapsed-time ticker prints periodic status messages during long-running engine compiles, server builds, and client builds (every 2 minutes). Prevents confusion during multi-hour builds with long silent periods (especially linking).
- [x] **Resume / incremental builds** — Cache miss reasons explain *why* a rebuild is happening ("no previous build recorded" or "inputs changed since last build"). Partial build detection checks for cooked content from a previous server/client build and suggests `--skip-cook` to skip re-cooking (saves 30-60 min). Wired into all build commands (`engine build`, `game build`, `game client`, `container build`) and the full pipeline.
- [x] **Build config guidance** — `ludus game build --build-config` help text and CLI output explain Shipping vs Development tradeoffs (binary size, debug symbols, optimization). Prints config note when `--build-config` is used. *(renamed from `--config` to avoid global flag conflict, PR #55)*

## Deploy UX

Smoothing out the deployment and testing workflow.

- [x] **Cost estimate before deploy** — All deploy commands and the pipeline print estimated hourly/monthly cost from a static pricing table before creating fleets. MCP results include `estimated_cost_per_hour` field.
- [x] **Auto-session (`--with-session`)** — `ludus deploy ec2 --with-session` that creates a game session immediately after the fleet goes active, saving a manual step.
- [x] **Batch destroy** — `ludus deploy destroy --all` iterates all 5 target types (gamelift, stack, ec2, anywhere, binary) and destroys resources for each, skipping targets that aren't deployed or fail gracefully.
- [x] **Instance type guidance** — Recommend instance types based on game characteristics (CPU-bound vs memory-bound) and provide cost/performance comparisons. `pricing.FormatGuidance()` shows a curated comparison table; `FormatSuggestion()` prints Graviton savings tips. Available in CLI deploy commands, pipeline, and MCP results.

## Diagnostics / Error Handling

Better observability and self-service troubleshooting.

- [x] **`ludus doctor` command** — 8 diagnostic checks: toolchain consistency (env vs engine requirement), stale build artifacts (>30 days), build state integrity (state.json cross-references), cache integrity, disk space (50 GB warn / 100 GB fail), AWS credential validity, Docker daemon status, git status. Platform-specific disk checks via build-tagged files.
- [x] **Guided error messages** — `internal/diagnose` package with table-driven error pattern matching. Three categories: AWS hints (12 patterns — expired tokens, access denied, quota limits, missing credentials), deploy hints (6 patterns — fleet errors, timeouts, conflicts, IP detection), container hints (6 patterns — disk full, daemon not running, ECR auth, rate limits). Wired into all deploy and container commands.

## Multi-Version UX

Better support for testing across multiple UE versions.

- [x] **`ludus config set` command** — `ludus config set key value` and `ludus config get key` for quick config updates from the CLI. Reads/writes `ludus.yaml` via Viper with type-aware value parsing. Creates the file if missing.
- [x] **State profiles** — `--profile <name>` flag on all commands. Default profile uses `.ludus/state.json` (backward compatible); named profiles use `.ludus/profiles/<name>.json`. Config is also profile-aware: `ludus --profile ue57 config set` writes to `ludus-ue57.yaml`. `ludus --profile ue57 run` loads `ludus-ue57.yaml` if it exists, with state isolated per profile. `state.ListProfiles()` and `state.DeleteProfile()` for profile management.

## Code Quality

Reducing duplication and improving maintainability.

- [x] **Dupl linter + refactor** — Enabled `dupl` linter (threshold 150) in `.golangci.yml`. Refactored: `saveClientState()` helper in `cmd/game`, `tryCreateSession()` and `checkCacheHit()` helpers in `cmd/mcp`, `runBatFile()` helper in `internal/engine`. Remaining structural duplicates (tags converters, native/Docker builder branches) are intentional — different types prevent meaningful abstraction.

## Security

Hardening generated artifacts and supply chain.

- [x] **Dockerfile security scanning** — `internal/dflint` package with 4 built-in rules (no-root-user, unpinned-base-image, no-package-cleanup, sensitive-env) + optional Hadolint and Trivy integration. Integrated into `ludus doctor` (game + engine Dockerfiles + container image scan), `ludus container build` (post-build lint), and the pipeline. Hadolint/Trivy are optional — gracefully skipped if not installed.
- [x] **Install hadolint and trivy locally** — hadolint v2.14.0 and trivy v0.69.3 installed via WinGet. `ludus doctor` runs extended Dockerfile lint and container image scans.
- [x] **Surface security findings in doctor + MCP** — `ludus doctor` now prints each hadolint/trivy finding (rule, severity, message) beneath summary lines. MCP `ludus_status` returns structured `security` array with findings so AI agents can see and act on vulnerabilities. *(PR #56)*

## Features

Larger feature additions from the project roadmap.

- [x] **ARM/Graviton support** — `--arch arm64` flag on `game build` and `deploy ec2` for cross-compiling LinuxArm64 servers targeting Graviton instances (20-30% cheaper). All Epic toolchain installers already ship the aarch64 sysroot. Config: `game.arch`, MCP: `arch` param on build/deploy tools. *(PR #36)*
- [x] **npm package for MCP distribution** — Publish `ludus-cli` npm package so AI agents (Claude Code, Cursor, etc.) can register Ludus as an MCP server via `npx ludus-cli mcp`. The npm `install.js` downloads the correct pre-built binary from GitHub Releases. Ties to GoReleaser — each tagged release produces binaries AND publishes the npm package. Users configure their AI agent with `{"command": "npx", "args": ["-y", "ludus-cli", "mcp"]}`.
- [x] **ARM64 container builds for GameLift** — `--arch arm64` flag on `container build` threads architecture through Dockerfile generation (correct base image variant + binary paths), wrapper binary cross-compilation, and `docker build --platform linux/arm64`. Auto-defaults GameLift/Stack instance types to Graviton when arch is arm64. Config: `game.arch`, CLI: `--arch`, MCP: `arch` param.
- [x] **Async MCP build tools** — Split long-running MCP tools (`ludus_game_build`, `ludus_game_client`, `ludus_engine_build`) into start/status pairs (`ludus_engine_build_start`, `ludus_game_build_start`, `ludus_game_client_start` return immediately with a build ID; `ludus_build_status` polls for completion, retrieves output, or cancels). Allows AI agents to start a build, do other work, and check back periodically instead of blocking for hours.
- [x] **BuildGraph XML generation** — `ludus buildgraph` command that generates BuildGraph XML from ludus.yaml config. Outputs a DAG of engine build (Setup → GenerateProjectFiles → Compile) and game build (BuildServer, optional BuildClient) stages. Config values become overridable `<Option>` elements. Consumable by Horde, UET, or any BuildGraph-compatible orchestrator. MCP tool: `ludus_buildgraph`. *(PR #57)*
- [ ] **Studio infrastructure provisioning** — Potentially a separate project that provisions game studio infrastructure on AWS (Perforce, CI/CD build farms, derived data cache, virtual workstations) as composable modules that integrate with Ludus. Decision point: integrate with AWS's [cloud-game-development-toolkit](https://github.com/aws-games/cloud-game-development-toolkit), wrap it, or build from scratch.
