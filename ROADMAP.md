# Ludus Roadmap

Prioritized list of planned work, organized by category. Items are roughly ordered by priority within each section.

## Stabilization

Bugs and rough edges discovered during cross-version E2E testing (UE 5.4–5.7 on Windows).

- [x] **UE 5.4 C4756 overflow patch** — `ludus init --fix` auto-patches UE 5.4 source files where NAN/INFINITY macros trigger C4756. Version-gated to 5.4 + SDK >= 26100. *(PR #35)*
- [x] **OOM detection / maxJobs halving for cross-compile** — `-j/--jobs` flag on `game build` and `game client`. Auto-detects RAM and halves parallelism for cross-compile (16GB/job vs 8GB/job). *(PR #35)*
- [x] **UAC failure detection** — Detects exit code `0xC0E90002` and provides 3 actionable remediation steps. *(PR #35)*
- [x] **Build failure diagnostics** — Scans RunUAT Log.txt for 7 known error patterns (missing content, OOM, DLL failures, NuGet, C4756, toolchain) with actionable hints. *(PR #35)*

## Onboarding / First-Run UX

Reducing friction for new users going from zero to a running game session.

- [ ] **`ludus setup` interactive wizard** — Guided first-run experience that: scans for engine source directories (e.g. `F:\Source Code\UnrealEngine-*`), auto-reads `Build.version` for the engine version, finds Lyra Launcher downloads in common paths (`Documents\Unreal Projects\LyraStarterGame*`), validates AWS credentials, and writes a complete `ludus.yaml`. Eliminates the need to manually create config.
- [x] **Auto-detect engine version** — Drop the `engine.version` config requirement. `toolchain.ParseBuildVersion()` already reads `Engine/Build/Build.version` JSON from every engine source tree. If version is empty in config, read it automatically.
- [x] **AWS credential validation** — `ludus init` checks `aws sts get-caller-identity` and warns if credentials aren't configured or expired. Warning-only so engine/game builds aren't blocked.
- [x] **"What's next" guidance** — After each command succeeds, print the next step in the pipeline. After `init`: "Run `ludus engine build`". After engine build: "Run `ludus game build`". After deploy: "Run `ludus deploy session`". Etc.
- [ ] **Lyra content auto-discovery** — Scan common paths (Epic Games Launcher vault cache, `Documents\Unreal Projects\LyraStarterGame*`) to auto-populate `game.contentSourcePath` in the setup wizard or suggest it during `ludus init`.
- [x] **Server map validation** — `ludus init` searches for `<serverMap>.umap` in the project's Content/ directory and warns if not found. Warning-only since maps can be path references or generated at cook time.

## Build UX

Improving the experience during long build operations.

- [ ] **Progress indicators** — Periodically tail the UBA log during builds and print "X/Y actions (Z%)" summaries. Even without a progress bar, periodic status updates are far better than hours of silence.
- [ ] **Resume / incremental builds** — Detect partial builds (e.g. from a previous OOM or crash) and offer to resume rather than restart from scratch. UBT already supports incremental builds; Ludus should surface this.
- [x] **Build config guidance** — `ludus game build --config` help text and CLI output explain Shipping vs Development tradeoffs (binary size, debug symbols, optimization). Prints config note when `--config` is used.

## Deploy UX

Smoothing out the deployment and testing workflow.

- [x] **Cost estimate before deploy** — All deploy commands and the pipeline print estimated hourly/monthly cost from a static pricing table before creating fleets. MCP results include `estimated_cost_per_hour` field.
- [x] **Auto-session (`--with-session`)** — `ludus deploy ec2 --with-session` that creates a game session immediately after the fleet goes active, saving a manual step.
- [x] **Batch destroy** — `ludus deploy destroy --all` iterates all 5 target types (gamelift, stack, ec2, anywhere, binary) and destroys resources for each, skipping targets that aren't deployed or fail gracefully.
- [ ] **Instance type guidance** — Recommend instance types based on game characteristics (CPU-bound vs memory-bound) and provide cost/performance comparisons.

## Diagnostics / Error Handling

Better observability and self-service troubleshooting.

- [ ] **`ludus doctor` command** — Comprehensive diagnostic tool (beyond `ludus status`) that checks for: stale DLLs, wrong toolchain version in env vs registry, disk space for upcoming builds, partial/corrupted build state, AWS credential expiry, common misconfigurations.
- [ ] **Guided error messages** — Every failure should tell the user exactly what to do next, not just what went wrong. Contextual fix suggestions based on exit codes, log patterns, and known issues per UE version.

## Multi-Version UX

Better support for testing across multiple UE versions.

- [x] **`ludus config set` command** — `ludus config set key value` and `ludus config get key` for quick config updates from the CLI. Reads/writes `ludus.yaml` via Viper with type-aware value parsing. Creates the file if missing.
- [ ] **State profiles** — Current single `state.json` is fragile for multi-version workflows. Support named state profiles or version-tagged state files natively, so switching between UE versions doesn't require manual state backup/restore.

## Code Quality

Reducing duplication and improving maintainability.

- [x] **Dupl linter + refactor** — Enabled `dupl` linter (threshold 150) in `.golangci.yml`. Refactored: `saveClientState()` helper in `cmd/game`, `tryCreateSession()` and `checkCacheHit()` helpers in `cmd/mcp`, `runBatFile()` helper in `internal/engine`. Remaining structural duplicates (tags converters, native/Docker builder branches) are intentional — different types prevent meaningful abstraction.

## Features

Larger feature additions from the project roadmap.

- [x] **ARM/Graviton support** — `--arch arm64` flag on `game build` and `deploy ec2` for cross-compiling LinuxArm64 servers targeting Graviton instances (20-30% cheaper). All Epic toolchain installers already ship the aarch64 sysroot. Config: `game.arch`, MCP: `arch` param on build/deploy tools. *(PR #36)*
- [ ] **BuildGraph XML generation** — `ludus buildgraph` command that generates BuildGraph XML validated against the UE schema. Outputs a ready-to-use XML file that UET, Horde, or other build orchestration tools can consume. An addition to the existing linear pipeline, not a replacement.
- [ ] **Studio infrastructure provisioning** — Potentially a separate project that provisions game studio infrastructure on AWS (Perforce, CI/CD build farms, derived data cache, virtual workstations) as composable modules that integrate with Ludus. Decision point: integrate with AWS's [cloud-game-development-toolkit](https://github.com/aws-games/cloud-game-development-toolkit), wrap it, or build from scratch.
