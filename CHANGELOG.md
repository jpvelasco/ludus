# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.1.4]: https://github.com/jpvelasco/ludus/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/jpvelasco/ludus/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/jpvelasco/ludus/releases/tag/v0.1.2
