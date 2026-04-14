# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- DDC (Derived Data Cache) support for container and WSL2 game builds — `ludus ddc` subcommand with `status`, `clean`, `prune`, and `warmup` commands (#151)
- WSL2 native build backend — `--backend wsl2` compiles engine and game servers directly inside a WSL2 distro, with optional `--wsl-native` ext4 fast path (#151)
- Automatic runtime dependency installation in WSL2 distros — `EnsureRuntimeDeps` installs libnss3, libdbus, and other UnrealEditor-Cmd requirements via `wsl.exe -u root`, with Ubuntu 24.04 t64 package fallback (#151)
- MCP tools for DDC management: `ludus_ddc_status`, `ludus_ddc_clean`, `ludus_ddc_prune`, `ludus_ddc_warm` (#151)
- Centralized UE5 dependency lists in `internal/dockerbuild/deps.go` — single source of truth for apt/dnf build and runtime packages (#151)
- Multi-stage engine Dockerfile with 5 stages (deps, source, generate, builder, runtime) and prebuilt variant for skip-engine mode (#151)

### Changed
- Promote Podman to recommended container backend in all help text, flag descriptions, and error messages (#151)
- E2E validated: DDC local mode delivers 16.6% cook-phase speedup on warm builds (311s cold, 260s warm — Lyra server, UE 5.7.4, WSL2/Podman)

### Fixed
- Fix macOS CI failure where docker-not-found crashed prereq checker instead of producing a warning (#151)
- Fix Codacy cyclomatic complexity and NLOC violations — extracted helpers across 8 files, split long test functions (#151)

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

[0.1.8]: https://github.com/jpvelasco/ludus/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/jpvelasco/ludus/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/jpvelasco/ludus/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/jpvelasco/ludus/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/jpvelasco/ludus/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/jpvelasco/ludus/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/jpvelasco/ludus/releases/tag/v0.1.2
