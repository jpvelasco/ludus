# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
