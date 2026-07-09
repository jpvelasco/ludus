# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.9.1] - 2026-07-08

**Patch release.** Significant test coverage improvements across MCP, setup, CLI, game, and pipeline packages, alongside AWS SDK dependency updates and documentation refreshes.

### Documentation

- **Polished README.** Updated hero section, DDC Zen details, observability and privacy sections, badge row with flat-square styling, and added Socket.dev security badge (#419).
- **Refreshed npm README.** Aligned npm landing page with v0.9.0 features: UE 5.4–5.8 support, 26 MCP tools, build backends, Zen DDC, ARM64/Graviton, build caching, OpenTelemetry, Account ID masking, `ludus doctor`, resource management, GameLift Anywhere, and VS Code Copilot MCP config (#435).

### Other

- **Test coverage sweep.** Added tests for MCP handlers (async builds, engine DDC, container, deploy, game, session, status), setup wizard helpers, CLI root setup/status, game builder helpers, and pipeline stages — pushing unit coverage toward 50% (#421, #422).
- **AWS SDK dependency bumps.** `aws-sdk-go-v2/service/s3` 1.104.0→1.104.2, `service/ecr` 1.58.4→1.58.6, `service/gamelift` 1.56.0→1.56.2, `config` 1.32.25→1.32.27 (#430–#433).
- **CI action bump.** `golangci/golangci-lint-action` 9.2.1→9.3.0 (#429).

## [0.9.0] - 2026-06-29

**Feature release.** Adds UE 5.8 support and headless-Windows build ergonomics, with a batch of deploy/cook fixes and a large test-coverage and complexity-reduction sweep.

### Added

- **UE 5.8 support.** The toolchain map resolves UE 5.8 to the v26 clang-20 cross-toolchain (shared with 5.7), and MSVC 14.44 is selected for 5.7+ Windows builds. Validated end-to-end with a live engine build and Lyra server cook (#399).
- **`--skip-setup` on `ludus engine build`.** Skips the `Setup.sh`/`Setup.bat` step when dependencies are already fetched, avoiding redist-installer hangs on headless Windows. Also exposed as `skip_setup` on the `ludus_engine_build` / `ludus_engine_build_start` MCP tools (native backend) (#412).
- **BuildSettingsVersion mismatch diagnostic.** Build failures from a stale `BuildSettingsVersion` now surface an actionable hint instead of a raw compiler error (#405).

### Fixed

- **Smart App Control false positive on Windows Server.** `init`/`doctor` no longer report SAC as enforcing when the `VerifiedAndReputablePolicyState` registry value is absent — an empty value is now correctly treated as off (#410).
- **VS 2022 Build Tools detection.** `vswhere` is now scoped with `-products *`, so a headless Build-Tools-only install is detected instead of reporting "no Visual Studio installation detected" (#411).
- **Self-contained server packaging.** Game server builds now stage with `-pak -iostore`, producing a self-contained Linux server (pak chunks + iostore) rather than loose cooked content (#406).
- **`DefaultServerTarget` anchoring.** The setting is written under the `[/Script/BuildSettings.BuildSettings]` section header rather than a project-derived target name (#404).
- **Connect staged-client discovery.** `ludus connect` discovers the staged client binary instead of guessing its name (#395).
- **Prebuilt-image pipeline checks.** Engine-source, toolchain, and disk prerequisites are skipped when building against a prebuilt engine image (#394).
- **GameLift Anywhere reliability.** Wrapper config, liveness handling, and the `make` prerequisite are fixed for the `anywhere` target (#392, #393).

### Changed

- **Codecov coverage with enforced 80% patch gate.** CI uploads coverage via OIDC and posts a `codecov/patch` status (soft block) on changed lines under 80%. A broad test-coverage sweep raised coverage across pipeline, prereq, game, container, setup, doctor, config, status, MCP, and CI packages (#378, #379–#391).
- **Complexity reduction.** High-complexity files were split into focused siblings and several functions simplified to satisfy Codacy's Lizard checks (#375, #376, #377, #403).

### Documentation

- **Aligned AGENTS.md and CLAUDE.md** with current code: DDC 5.4+, 26 MCP tools, Go 1.25.11, `ludus.yaml` config precedence, plus coverage and test-gate guidance (#416).

### Other

- Dependency and CI action bumps: `smithy-go` 1.27.2→1.27.3, `actions/setup-go` 6.4.0→6.5.0, `goreleaser-action` 7.2.2→7.2.3 (#400, #401, #402).

## [0.8.2] - 2026-06-27

**Patch release.** Centralizes AWS account/region resolution and ECR URI construction, with follow-up fixes across deploy, setup, and MCP paths.

### Changed

- **Centralized AWS environment resolution.** Account ID, region, and ECR image/repository/registry URIs now resolve through `internal/awsenv` instead of duplicated per-command logic. Affects ECR push, container build, deploy (all targets), MCP tools, and the pipeline. Eliminates inconsistent URI construction and scattered STS/IMDS lookups (#367).

### Fixed

- **Container group definition updates.** `CreateContainerGroupDefinition` now deletes a stale definition when image, port, or config mismatches and retries create, instead of failing on an incompatible existing definition (#367, #370).
- **Setup wizard AWS detection.** `ludus setup` uses the shared `awsenv` resolver (with IMDS timeout) instead of a direct `aws sts` subprocess (#370).
- **Deploy and MCP error propagation.** Stack flag resolution errors, missing ECR repo/tag validation, and context propagation fixes ensure AWS resolution failures surface correctly across deploy, pipeline container-push, and MCP destroy paths (#367).

## [0.8.3] - 2026-06-27

**Patch release.** Removes a 59 MB stale test binary from the repo and tightens `.gitignore` to prevent recurrence.

### Fixed

- **Stale `ludus-test` binary removed from repo.** The 59 MB test binary (produced by `go test -c`) was accidentally committed in PR #368 because `.gitignore` only covered `ludus`, `ludus.exe`, and `ludus-*.exe` — not extensionless test binaries. Added `ludus-test` to `.gitignore` to prevent recurrence (#373).

## [0.8.1] - 2026-06-23

Build-host ergonomics and a batch of backlog fixes.

### Fixed
- **`ludus engine build --jobs 0` now actually auto-detects compile parallelism** from the host (CPU cores and RAM), instead of silently using a hardcoded `MAX_JOBS=4`. The flag's help has always advertised auto-detection; now it's real. Auto-detect uses `min(NumCPU, RAM_GB / 2)` so large build hosts use their cores while RAM still bounds parallelism to avoid OOM on memory-heavy UE translation units. An explicit `--jobs N` / `engine.maxJobs` is still honored as-is; detection falls back to the previous default of 4 when host resources can't be read (#350).
- Container game builds against a **prebuilt engine image** (`engine.dockerImage`) no longer fail the "Engine Source" prerequisite. The build runs inside the image and doesn't read the host engine source tree, so that check is skipped when a prebuilt image is configured (#361).
- Compile-job auto-detection now works on **macOS**, reading total RAM via `sysctl hw.memsize` instead of the Linux-only `/proc/meminfo` (which returned 0 on macOS, defeating detection) (#364).
- `ludus container push` now returns an **actionable error** when the AWS identity lacks `ecr:CreateRepository` (e.g. `AmazonEC2ContainerRegistryPowerUser`), pointing to pre-creating the repository or granting the action, instead of a raw `AccessDenied` (#362).

### Documentation
- Documented the npm `allow-scripts` install warning as benign (the binary self-heals on first run) and how to silence it, in the npm package README (#358).
- Noted that `ludus container push` auto-create needs `ecr:CreateRepository`, with guidance for least-privilege/CI roles (#362).

## [0.8.0] - 2026-06-23

Makes `ludus deploy destroy` safe by default and replaces the overloaded `--all` flag.

### Changed (breaking)
- **`ludus deploy destroy` no longer deletes ECR repositories or S3 build buckets by default.** The default now removes only the active target's *ephemeral* resources (fleet, container group definition, IAM role, EC2 build + its S3 object) and preserves durable build artifacts. The old `--all` flag is removed and replaced by two independent flags: `--all-targets` (tear down every target type) and `--purge` (also delete durable artifacts — ECR repos + S3 build buckets, with a `[y/N]` confirmation unless `--yes`). The CLI and the `ludus_deploy_destroy` MCP tool now behave consistently. Previously the default `destroy` silently deleted the ECR repository for container targets, which could wipe expensive build images (#357).

## [0.7.3] - 2026-06-23

Fixes container/deploy path resolution for projects whose name differs from the `.uproject`, and DDC path resolution when `$HOME` is unset.

### Fixed
- Derive the packaged content directory name from the `.uproject` filename instead of `game.projectName`. UE names the staged content dir after the `.uproject` (e.g. `LyraStarterGame6/`), so container builds and the anywhere/ec2 deployers failed to find the server binary when `projectName` differed from the `.uproject` name. `projectName` is now only a default for target names.
- `ludus ddc status` (and DDC path resolution generally) no longer errors with "resolving home directory" when `$HOME` is unset (e.g. under SSM Run Command / some CI). Falls back to the user database, then `/root` on \*nix.

### Documentation
- Clarified `game.projectPath` vs `game.projectName` vs `game.serverTarget` semantics in the README, `ludus.example.yaml`, and config docs: `projectPath` (the `.uproject`) is the source of truth for paths; `projectName` only defaults target names.

## [0.7.2] - 2026-06-23

Completes the Zen DDC container fix so cooks can actually use the Zen Store.

### Fixed
- Recursively chown the ZenStore mount tree (`/home/ue/.config`) in the game-build preamble. The v0.7.1 fix chowned only the top two `.config` levels, leaving the deeper Docker-created mount path (`.../Common/Zen/Data`) and the host-backed Zen dir root-owned. `zenserver` (running as `ue`) then failed its readiness check (`errno=13` writing the Zen Data store / `Zen/Install`), the DDC backend graph had no writable node, and the cook crashed with `Signal 11` (`ExitCode=139`). Found during live UE 5.7.4 + Lyra container validation (#353).

## [0.7.1] - 2026-06-23

Fixes a container game-build permission failure on engine images that ship a pre-existing build user.

### Fixed
- Recursively chown `/project` in the game-build preamble's pre-existing-`ue` branch. Previously it chowned `/project` non-recursively, leaving root-owned subdirectories (e.g. `Config/`); UAT then failed setting `DefaultServerTarget` with `sed: couldn't open temporary file /project/Config/...: Permission denied`. Same failure family as #340, on a different path. Found during live UE 5.7.4 + Lyra container validation.

## [0.7.0] - 2026-06-22

Makes the Unreal Zen Store the default DDC backend across all supported UE versions and fixes a v0.6.0 container-build regression.

### Changed
- **Zen Store is now the default DDC backend.** `ddc.mode` defaults to `zen` (the Unreal Zen Store), UE's default local DDC backend since UE 5.4. Applies to all supported engine versions (5.4–5.7). Container builds mount the host Zen directory (`ddc.zenPath`, default `~/.ludus/zen`); native and WSL2 builds use UE's own Zen Store, which already persists. The earlier "ZenStore on UE 5.6+ only" assumption was incorrect and has been corrected throughout the code and docs.
- **`ddc.mode: local` (legacy FileSystem DDC) is deprecated.** Still honored, but `ludus doctor`, `ludus ddc status`, and config load now warn that it is delete-only in UE since 5.4 and recommend `zen`.

### Fixed
- Chown the Docker-created ZenStore mount parents (`/home/ue/.config`, `/home/ue/.config/Epic`) so container game builds with a Zen DDC mount no longer fail with "Access to /home/ue/.config/Unreal Engine denied" (#340)

### Other
- Bump AWS SDK dependencies (config, gamelift, s3, iam) and `actions/checkout` to 7.0.0 (#341–345)

## [0.6.0] - 2026-06-22

Adds ARM64/Graviton dedicated server builds, build observability (on-disk logs + optional OpenTelemetry tracing), and AWS account ID masking, alongside a broad set of container-build reliability fixes.

### Added
- **ARM64 / Graviton server builds.** Build Linux dedicated servers for `arm64` with UE dependent platforms, deployable to Graviton fleets (#303)
- **Build observability.** Build output is teed to per-run log files under `.ludus/logs/` (queryable via `ludus logs list|path|tail`), with optional OpenTelemetry (OTLP) trace export emitting one span per pipeline stage (#334)
- **AWS account ID masking.** 12-digit account IDs in ECR URIs and ARNs are masked in terminal output by default; override with `--show-account-id` or `privacy.maskAccountId` (JSON/MCP output unaffected) (#336)
- **ZenStore DDC persistence (UE 5.6+).** Cook DDC written to ZenStore is persisted across container game builds via `ddc.zenPath`, restoring DDC reuse on UE 5.6+ (#330)

### Fixed
- Backend-aware disk validation enforces the 1 TB minimum for container builds (#333)
- Auto-derive the AWS account ID and region in engine/container push (#322)
- Prune BuildKit cache after the engine build to reclaim ~200 GB of disk (#323)
- Wire `OutputDir` in `BaseDockerGameOptions` to prevent a build path mismatch (#328)
- Fast-fail the container runtime check before a game container build (#309)
- Add an Xcode prerequisite check on macOS for native builds (#310)
- Fail fast when QEMU amd64 is not registered for arm64 Linux container builds (#321)
- Reliably set `DefaultServerTarget` in the ini for multi-target projects (#326)
- Use `dotnet-install.sh` for ARM64 engine container builds (#312)
- `chmod` temp build scripts to 0644 for container non-root access (#318)
- `chown` engine parent directories for container game builds (#299)
- Apply game arch overrides before caching so the cache key is correct (#302)
- Normalize the default container engine max jobs (#301)
- Correct engine push image references (#300)
- Self-healing npm install/upgrade for `ludus-cli` on skipped postinstall, failed download, or version skew (#335)
- Don't assert no-QEMU on Apple Silicon in the cross-arch native test (#332)

### Documentation
- Document the `ludus logs` command, `observability` / `ddc.zenPath` config, and module map (#338)
- Clarify that `game.projectPath` must be the `.uproject` file path, not a directory (#329)
- Document release-process guardrails — CHANGELOG-first, OIDC publishing, immutable tags (#337)
- Update container-build disk requirements to 1 TB (#317)
- Clarify that QEMU engine builds are impractical on Apple Silicon (#311)

### Dependencies
- Bump `aws-sdk-go-v2/service/{sts,ecr,s3,gamelift}` (#305, #308, #306, #304)
- Bump `github.com/aws/smithy-go` 1.27.1 → 1.27.2 (#307)

### Other
- Untrack private design docs from the public repo (#288)
- Repo housekeeping: gitignore agent cache, add DDC docs (#287)

## [0.5.1] - 2026-06-12

### Added
- `ludus doctor` checks Go toolchain version for container builds, warning when the host Go version is too old for the wrapper cross-compile step (#274)

### Fixed
- `--dry-run` no longer records build cache entries, so a subsequent real build is not incorrectly skipped (#273)
- Container game builds derive the `.uproject` filename from the project path instead of `projectName`, fixing BYO projects whose filename differs from the target prefix (e.g. `LyraStarterGame.uproject` with `projectName: Lyra`) (#271, #278)
- macOS preflight installs `ca-certificates` so HTTPS downloads (e.g. `wget`) succeed inside the Linux build container (#269, #280)
- arm64 server cooks use `-platform=Linux` with `TargetArchitecture=AArch64` INI instead of `-platform=LinuxArm64`, fixing `Invalid target platform LinuxArm64Server` cook failures (#283)
- `ludus deploy destroy` no longer deletes the `ludus-engine` ECR repository — engine images are build inputs, not deployment artifacts (#284)

### Changed
- Extract WSL2 game build setup into `setupWSL2GameBuild`, reducing cyclomatic complexity of `handleWSL2GameBuild` (#285)

### Security
- Bump Go toolchain to 1.25.11 to address stdlib CVEs (CVE-2026-27145, CVE-2026-42507)

### Dependencies
- Bump `github.com/aws/aws-sdk-go-v2/config` 1.32.18 → 1.32.23
- Bump `github.com/aws/aws-sdk-go-v2/service/cloudformation` 1.71.13 → 1.72.1
- Bump `github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi` 1.32.2 → 1.33.3

## [0.5.0] - 2026-06-05

### Added
- **macOS Support in Preview** (container backends on Apple Silicon and Intel)
  - Full end-to-end pipeline support for Linux dedicated servers from macOS using `--backend docker` or `--backend podman` (the primary/recommended path; native mac builds target macOS, not Linux).
  - Engine container builds always force `linux/amd64` (QEMU user-mode emulation required; Epic ships only an x86_64 Linux toolchain). One-time Linux toolchain bootstrap + GenerateProjectFiles via throwaway Linux container (cached; ~2 GB noted in progress + doctor checks). Use pre-built engine image (`engine.dockerImage` in ludus.yaml) to skip repeated QEMU cost.
  - Game builds with `--arch arm64` (Graviton) supported via cross-compilation inside the emulated amd64 environment, producing correct `LinuxArm64Server/` / `LinuxArm64/` output and binaries.
  - `ludus doctor` + prereqs now include Apple Silicon + container specific checks/warnings: "container backend on Apple Silicon: engine + game builds use QEMU x86_64 emulation (due to Epic's toolchain). game.arch=arm64 still produces correct Graviton server output via cross-compilation. Emulation has a performance cost. Recommended: pre-build engine on x86_64 Linux + registry for speed."
  - See new dedicated "macOS Support" section in README (prereqs, recommended Graviton workflow, full command examples with --backend/--arch, config snippet, doctor note) + ARCHITECTURE.md updates. Added design spec + implementation plan docs.

  **Preview / Experimental**: More real-world testing on M-series Macs is still needed. Emulation has a performance cost; engine images remain amd64 even for arm64 game output.

### Fixed
- `ludus container build --dry-run` succeeds cleanly after printing commands (no longer attempts wrapper template read or staging when dry-run mode) (#261)
- Install `dotnet-sdk-8.0` (plus Microsoft repo for apt on Ubuntu 22.04; dnf path) in container base images to fix UBT "System.Runtime.Numerics not found" on Ubuntu 22.04 (#252, closes #249)
- `ludus setup` wizard now pre-fills prompts from existing config and preserves fields not prompted in current run (#247)
- Multiple Codacy complexity / NLOC / lint issues introduced during macOS work (extracts in engine builder, doctor test, os checker test, runSetup, cross-arch) (#254, #259, #262)

### Changed
- macOS container stabilization Phase 1 (closes #243 + related #237–#240): engine force to linux/amd64 for containers + preflights (darwin + container), full `game.arch` support (LinuxArm64Server output, -platform, INI TargetArchitecture) in DockerGameBuilder + scripts + results, dnf/apt preflight support, arch threading in pipeline/MCP/cache, Apple Silicon warnings in prereq + doctor (with platform output), macOS preflight helper extract, test coverage (table-driven for amd64 force + arm64 results), docs. (#244, #245, #246, #248, #250, #251, #253, #255, #256, #257, #259)
- Pre-release regression + CI/Lint(Windows) validation pass: no regressions on Windows native, WSL2 (engine+game dry + units), Linux container paths; doctor/setup/status/deploy fallbacks; all CI (incl. Lint(Windows)) confirmed stable (#262)
- Setup, complexity reductions, and maintenance from macOS series work.

### Documentation
- Added "macOS Support" section (README) with practical instructions, examples, and Graviton workflow. Final polish to Getting Started + ARCHITECTURE.md cross-arch notes. Community files moved to .github/ + repo settings reference (#223 + mac docs PRs).

### Internal / Maintenance
- Added table-driven tests for macOS/container behaviors (engine amd64 platform force on docker/podman; game arm64 results + platform in container path) (#256, #262).
- CI now includes macos-latest in build/test matrix (race on mac).
- Refactors/extracts for complexity in touched macOS paths (engine, doctor, prereq, setup).

### Dependencies
- Bump github.com/aws/aws-sdk-go-v2/service/s3 (#233)
- Bump github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi (#231)
- Bump github.com/aws/smithy-go (#232)
- Bump github.com/aws/aws-sdk-go-v2/config and other AWS SDKs (dependabot)
- Bump modelcontextprotocol/go-sdk (#221)
- CI action bumps (checkout, golangci-lint-action, goreleaser-action) (#219, #220, #229)

## [0.4.2] - 2026-05-14

### Fixed
- Resolve DDC config before launching async goroutines in `ludus_game_build_start` and `ludus_game_client_start`, preventing a data race against global config (#217)

### Security
- Bump Go toolchain to 1.25.10 to address 5 HIGH CVEs in `golang/stdlib` (#213)
- Bump `github.com/aws/aws-sdk-go-v2/service/s3` 1.100.1 → 1.101.0 (#203)

### Changed
- Extract helpers to reduce cyclomatic complexity and function length in `cmd/game`, `cmd/deploy`, `cmd/mcp`, and `cmd/globals` (#214, #215, #216)
- Add unit tests for `recordFleetDeployState`, `buildWSL2GameOptions`, `resolveWSL2GameDDCPath`, `ResolveSessionTarget`, and native async build starters (#217)

## [0.4.1] - 2026-05-13

### Changed
- Update Go module path from `github.com/devrecon/ludus` to `github.com/jpvelasco/ludus` (#209)

  Binary users (npm, GitHub Releases) are unaffected. Go module users should run:
  ```
  go mod edit -module github.com/jpvelasco/ludus
  go mod tidy
  ```

## [0.4.0] - 2026-05-14

### Added
- `ludus_engine_build_start` and `ludus_game_build_start` MCP tools now support `backend=wsl2`, returning a build ID immediately and running the WSL2 build asynchronously (#207)

### Fixed
- `ludus_deploy_fleet` MCP tool was deploying to the binary target instead of GameLift due to missing target resolution (#206)
- `ludus_deploy_session` and `ludus run` session step now fall back to `state.deploy.targetName` when the config target doesn't support sessions (#206)
- GameLift fleet and container deployments no longer fail to write `deploy.targetName` to state, fixing subsequent `ludus_deploy_session` calls (#205)
- WSL2 game server build archived to wrong output path (#204)
- `ludus container build` was missing `--backend` flag (hardcoded Docker) (#204)
- Podman builds no longer fail with `--provenance=false` (Docker-only BuildKit flag) (#204)
- `ludus_container_build` MCP tool now accepts a `backend` parameter (#204)

### Changed
- Refactor Codacy maintainability hotspots by extracting polling, deployment cleanup, build graph, workflow, and template helpers.
- Split multiple large source files into focused domain files across cache, config, state, runner, pipeline, game, gamelift, ec2fleet, stack, and awsutil packages (#183–#202)
- Split oversized test files into focused test files across all packages (#187–#193)

## [0.2.2] - 2026-05-05

### Fixed
- Patch CVE-2026-32283: upgrade Go toolchain to 1.25.9 (TLS 1.3 deadlock, HIGH severity) (#178)

### Dependencies
- Bump github.com/aws/aws-sdk-go-v2/service/cloudformation from 1.71.9 to 1.71.11 (#173)
- Bump github.com/aws/aws-sdk-go-v2/service/ecr from 1.56.2 to 1.57.2 (#174)
- Bump github.com/aws/aws-sdk-go-v2/service/gamelift from 1.52.0 to 1.54.0 (#175)
- Bump github.com/modelcontextprotocol/go-sdk from 1.4.1 to 1.6.0 (#176)
- Bump github.com/aws/aws-sdk-go-v2/config from 1.32.14 to 1.32.17 (#177)

## [0.2.1] - 2026-05-01

### Dependencies
- Bump github.com/aws/aws-sdk-go-v2/service/sts from 1.41.10 to 1.42.1 (#169)
- Bump github.com/aws/aws-sdk-go-v2/service/iam from 1.53.7 to 1.53.10 (#168)
- Bump github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi from 1.31.10 to 1.31.12 (#167)
- Bump github.com/aws/aws-sdk-go-v2/service/s3 from 1.99.0 to 1.100.1 (#166)
- Bump github.com/aws/aws-sdk-go-v2 core from 1.41.5 to 1.41.7 (transitive, supersedes #170)
- Bump github.com/aws/smithy-go from 1.24.3 to 1.25.1 (transitive)
- Bump actions/setup-node from 6.3.0 to 6.4.0 (#164)
- Bump goreleaser/goreleaser-action from 7.0.0 to 7.2.1 (#171)

## [0.2.0] - 2026-04-15

### Added
- **DDC Support** (`ludus ddc` commands + `--ddc local` flag)
  - Persistent Derived Data Cache across builds
  - Up to **59% faster cook times** on warm Zen cache (true cold benchmark)
  - Subcommands: `status`, `clean`, `prune`, `warmup`
  - MCP tools: `ludus_ddc_status`, `ludus_ddc_clean`, `ludus_ddc_configure`, `ludus_ddc_warm`
- **WSL2 Native Build Backend** (`--backend wsl2`)
  - Fast native ext4 builds, bypassing Docker/Podman virtiofs bottlenecks
  - Two modes: default (virtiofs) + `--wsl-native` (rsync to native ext4)
  - Full pipeline integration: `ludus run --backend wsl2 [--wsl-native]`
  - MCP integration: `ludus_engine_build` and `ludus_game_build` accept `backend=wsl2`
  - Automatic runtime dependency installation (`libnss3`, `libdbus`, etc.) via `wsl.exe -u root`
- Full integration of DDC with WSL2 native path
- Centralized UE5 dependency lists in `internal/dockerbuild/deps.go` — single source of truth for apt/dnf packages
- Multi-stage engine Dockerfile with 5 stages and prebuilt variant for `--skip-engine` mode
- CODEOWNERS file assigning `@jpvelasco` as default code owner

### Changed
- `ludus game build` and `ludus run` now support `--backend wsl2 --ddc local`
- Promote Podman to recommended container backend in all help text and error messages
- Improved runtime dependency handling for WSL2 and container builds (Ubuntu 24.04 t64 package fallback)

### Fixed
- Fix macOS CI failure where docker-not-found crashed prereq checker instead of producing a warning
- Fix Codacy cyclomatic complexity and NLOC violations — extracted helpers across 8 files

### Benchmarks
- True cold vs warm DDC test (Lyra, x86_64, WSL2 native):
  - Cook: 1321s → **541s** (**59% faster**)
  - Full BuildCookRun: 2205s → 1160s (**47% faster**)

### Dependencies
- Bump github.com/aws/smithy-go from 1.24.2 to 1.24.3 (#156)
- Bump github.com/aws/aws-sdk-go-v2/config from 1.32.12 to 1.32.14 (#153)
- Bump github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi from 1.31.9 to 1.31.10 (#152)
- Bump github.com/aws/aws-sdk-go-v2/service/s3 from 1.97.3 to 1.98.0 (#154)
- Bump github.com/aws/aws-sdk-go-v2/service/gamelift from 1.51.1 to 1.52.0 (#155)

## [0.1.17] - 2026-04-02

### Fixed
- Fix Anywhere wrapper binary hardcoded to Linux — now builds for host OS on Windows hosts (#148)
- Fix global config mutation via pointer copy in isolated config helper (#143)
- Fix deploy destroy to clean up ECR repositories (#142)

### Changed
- Reduce complexity violations across production and test code — 43 down to 14 Lizard violations (#136, #137, #138, #139, #140, #141, #146)
- Exclude non-application paths (npm/, scripts/) from all Codacy tools (#149)
- Pin GitHub Actions to commit SHAs and tune Codacy config (#134)

### Dependencies
- Bump Go 1.25.8 — 3 CVE fixes (#137)
- Bump AWS SDK v2: STS, CloudFormation, S3, IAM, ECR (#129, #130, #131, #132, #133)

## [0.1.16] - 2026-03-30

### Fixed
- Fix game client connection args — use travel URL as first positional arg instead of `-connect=` flag that Lyra ignores (#127)
- Fix GameLift container fleet and CloudFormation stack port mapping — remove manual `InstanceInboundPermissions` so GameLift auto-calculates the optimal public port range, avoiding restricted ports 4080/5757 (#127)
- Fix Anywhere deployer hardcoded `Binaries/Linux/` path — now uses `runtime.GOOS`/`GOARCH` to resolve the correct platform binary on any host OS (#126)
- Fix binary exporter using raw `exec.Command("cp")` — replaced with pure Go `copyDir()` for cross-platform support and `--dry-run` compliance (#125)

### Changed
- Extract shared AWS utilities into `internal/awsutil` — `LoadAWSConfig` and `IsNotFound` consolidated from 5 packages (#120)
- Consolidate `GameSessionInfo` into `deploy.SessionInfo` and extract `ResolveServerBuildDir` into `internal/config` (#121)
- Extract instance type auto-switch into `pricing.AutoSwitch` (#122)
- Deduplicate runner echo+dry-run block into `echo` helper method (#123)
- Extract shared ECR push logic into `internal/ecr` (#124)
- Add `.codacy.yml` for Codacy static analysis configuration (#127)

### Added
- Unit tests for connect args, GameLift fleet input, CloudFormation template, binary exporter, game builder helpers, prereq checks, and status checks (#125, #127)

## [0.1.15] - 2026-03-26

### Fixed
- Fix Ctrl+C during batch file execution on Windows — no more "Terminate batch job (Y/N)?" prompt leaking to PowerShell, no orphan processes surviving after exit (#118)
- Add `signal.NotifyContext` to root command for proper context cancellation on interrupt (#118)

## [0.1.14] - 2026-03-26

### Added
- `ludus resources` command — scans AWS for all ludus-managed resources using the Resource Groups Tagging API and known naming patterns (#114)
- `ludus deploy destroy --all` now cleans up ECR repositories and S3 build buckets in addition to deploy targets (#114)
- MCP tool `ludus_resources` for resource inventory, and `all` flag on `ludus_deploy_destroy` (#114)

### Fixed
- Tag ECR repositories with `ManagedBy=ludus` on creation — previously untagged, making them invisible to resource discovery (#115)
- Resource type label for GameLift container fleets (`containerfleet` ARN type) (#114)

## [0.1.13] - 2026-03-25

### Added
- Lightweight prerequisite checks for individual stage commands — `engine build`, `game build`, `container build/push`, `deploy *`, and `connect` now validate only the prerequisites relevant to that command before running (#112)

## [0.1.12] - 2026-03-25

### Fixed
- Add cross-architecture emulation check to prerequisites — detects missing QEMU/binfmt early in `ludus init` and `ludus run` instead of failing during container build (#108)
- Check Docker daemon is running in prerequisites — previously only verified the binary was in PATH (#109)
- Search Plugins directory for server map in prerequisites — UE5 GameFeature plugin maps like `L_Expanse` are now found correctly (#110)

### Dependencies
- Bump `aws-sdk-go-v2/service/s3` from 1.96.4 to 1.97.1 (#102)
- Bump `aws-sdk-go-v2/service/cloudformation` from 1.71.7 to 1.71.8 (#103)

## [0.1.11] - 2026-03-23

### Fixed
- `ludus run` now auto-fixes prerequisite issues (e.g. missing plugin DLLs) instead of failing with no guidance — previously only `ludus init --fix` could resolve them (#106)

## [0.1.10] - 2026-03-23

### Fixed
- Suppress sensitive AWS output (account IDs, tokens) from `container push` and `engine push` commands — new `RunQuiet`/`RunQuietWithStdin` runner methods suppress stdout unless `--verbose` is set (#104)

## [0.1.9] - 2026-03-23

### Fixed
- Toolchain detection when `LINUX_MULTIARCH_ROOT` points to the toolchain directory itself — Epic's official installer sets the env var to the full toolchain path (e.g. `v26_clang-20.1.8-rockylinux8/`) rather than its parent; `findToolchainInRoot()` now checks subdirectories, the root itself, and sibling directories (#100)
- Pre-existing SA5011 staticcheck false positives in buildgraph and toolchain tests

## [0.1.8] - 2026-03-21

### Fixed
- npm publish now works with OIDC trusted publishing — v0.1.6 and v0.1.7 npm releases were skipped due to Node 22 shipping npm 10.x which lacks OIDC support; upgraded to Node 24 (npm 11.x) (#98)

## [0.1.7] - 2026-03-21

### Changed
- Switch npm publish to OIDC trusted publishing — no more stored npm tokens (#96)

## [0.1.6] - 2026-03-21

### Added
- Detailed licensing notes and UE5 compliance disclaimer to README and npm docs (#94)
- Unit tests for cache, diagnose, dockerfile, progress, runner, status, and wrapper packages

### Dependencies
- aws-sdk-go-v2/service/gamelift 1.50.2 → 1.51.1 — adds DDoS protection for Linux EC2 and Container fleets (#89)
- modelcontextprotocol/go-sdk 1.4.0 → 1.4.1 — security patch for JSON parsing and cross-origin protection (#90)
- aws-sdk-go-v2/service/sts 1.41.8 → 1.41.9 (#91)
- aws-sdk-go-v2/service/iam 1.53.5 → 1.53.6 (#92)
- aws-sdk-go-v2/config 1.32.11 → 1.32.12 (#93)

## [0.1.5] - 2026-03-11

### Added
- Claude Code MCP configuration to README and npm docs (#85)

## [0.1.4] - 2026-03-11

### Added
- Retry with exponential backoff for network-dependent CLI commands — docker push, ECR auth, git clone, and curl downloads now retry up to 3 times with jitter (#83)
- Go Report Card and npm version badges in README (#81)
- Quickstart section in README (#81)
- Unit tests for config, container, state, tags, pricing, and cache (#80)

### Fixed
- Disk space requirement updated from 100 GB to 300 GB to reflect actual UE5 engine size (#81)
- Documentation inaccuracies in AGENTS.md and README.md (#74)

### Changed
- Node.js 20 to 22 LTS in release workflow (#82)

### Dependencies
- aws-sdk-go-v2/config 1.32.10 → 1.32.11 (#75)
- aws-sdk-go-v2/service/cloudformation bump (#76)
- aws-sdk-go-v2/service/s3 1.96.2 → 1.96.4 (#78)
- aws-sdk-go-v2/service/iam bump (#79)

## [0.1.3] - 2026-03-08

### Added
- Keywords to npm package for discoverability (#73)

## [0.1.2] - 2026-03-08

Initial public release.

### Highlights
- Full pipeline in one command — `ludus run` orchestrates engine build, game server packaging, containerization, ECR push, and GameLift fleet deployment
- 5 deployment targets — GameLift Containers, CloudFormation Stack, Managed EC2, GameLift Anywhere (local dev), and binary export
- ARM64 / Graviton support — cross-compile and deploy to Graviton instances
- Docker build backend — build UE5 inside Docker for reproducible CI builds
- Build caching — input-hash-based caching skips unchanged stages
- MCP server — 21 tools for AI agent orchestration
- GameLift Anywhere — local development mode
- BuildGraph XML generation — for Horde/UET CI pipelines
- npm package (`ludus-cli`) with pre-built binaries for Linux, macOS, and Windows

[0.4.1]: https://github.com/jpvelasco/ludus/compare/v0.4.0...v0.4.1
[0.2.1]: https://github.com/jpvelasco/ludus/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/jpvelasco/ludus/compare/v0.1.17...v0.2.0
[0.1.8]: https://github.com/jpvelasco/ludus/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/jpvelasco/ludus/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/jpvelasco/ludus/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/jpvelasco/ludus/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/jpvelasco/ludus/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/jpvelasco/ludus/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/jpvelasco/ludus/releases/tag/v0.1.2
[Unreleased]: https://github.com/jpvelasco/ludus/compare/v0.9.1...HEAD
[0.9.1]: https://github.com/jpvelasco/ludus/releases/tag/v0.9.1
[0.9.0]: https://github.com/jpvelasco/ludus/compare/v0.8.3...v0.9.0
[0.8.3]: https://github.com/jpvelasco/ludus/releases/tag/v0.8.3
[0.8.2]: https://github.com/jpvelasco/ludus/compare/v0.8.1...v0.8.2
[0.8.1]: https://github.com/jpvelasco/ludus/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/jpvelasco/ludus/compare/v0.7.3...v0.8.0
[0.7.3]: https://github.com/jpvelasco/ludus/compare/v0.7.2...v0.7.3
[0.7.2]: https://github.com/jpvelasco/ludus/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/jpvelasco/ludus/compare/v0.7.0...v0.7.1
[0.7.0]: https://github.com/jpvelasco/ludus/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/jpvelasco/ludus/compare/v0.5.1...v0.6.0
[0.5.0]: https://github.com/jpvelasco/ludus/compare/v0.4.2...v0.5.0
[0.4.2]: https://github.com/jpvelasco/ludus/compare/v0.4.1...v0.4.2
