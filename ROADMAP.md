# Ludus Roadmap

Prioritized list of planned work, organized by category. Items are roughly ordered by priority within each section.

## Stabilization

Bugs and rough edges discovered during cross-version E2E testing (UE 5.4–5.7 on Windows).

- [ ] **UE 5.4 C4756 overflow patch** — MSVC 14.38 + Windows SDK 26100 triggers `C4756` (overflow in constant arithmetic) treated as error in UE 5.4. Needs a version-gated auto-patch in `ludus init --fix`, similar to the existing INITGUID fix for 5.6.
- [ ] **OOM detection / maxJobs halving for cross-compile** — Cross-compilation loads both Win64 and Linux toolchains simultaneously; `maxJobs` auto-detect (based on RAM / 8 GB) should halve during game server builds to avoid OOM. Currently users must manually set `engine.maxJobs` lower.
- [ ] **UAC failure detection** — Content cooking silently fails with exit code `0xC0E90002` when UAC blocks a subprocess. Need to detect this specific exit code and provide actionable guidance (run as administrator or adjust UAC settings).
- [ ] **Build failure diagnostics** — Parse cook and UBA logs for common failure patterns (missing content, DLL load errors, OOM kills, missing SDK components) and surface actionable fix suggestions instead of generic "ExitCode=25" errors.

## Onboarding / First-Run UX

Reducing friction for new users going from zero to a running game session.

- [ ] **`ludus setup` interactive wizard** — Guided first-run experience that: scans for engine source directories (e.g. `F:\Source Code\UnrealEngine-*`), auto-reads `Build.version` for the engine version, finds Lyra Launcher downloads in common paths (`Documents\Unreal Projects\LyraStarterGame*`), validates AWS credentials, and writes a complete `ludus.yaml`. Eliminates the need to manually create config.
- [ ] **Auto-detect engine version** — Drop the `engine.version` config requirement. `toolchain.ParseBuildVersion()` already reads `Engine/Build/Build.version` JSON from every engine source tree. If version is empty in config, read it automatically.
- [ ] **AWS credential validation** — `ludus init` should check `aws sts get-caller-identity` and warn if credentials aren't configured or have expired, before the user gets deep into a multi-hour pipeline.
- [ ] **"What's next" guidance** — After each command succeeds, print the next step in the pipeline. After `init`: "Run `ludus engine build`". After engine build: "Run `ludus game build`". After deploy: "Run `ludus deploy session`". Etc.
- [ ] **Lyra content auto-discovery** — Scan common paths (Epic Games Launcher vault cache, `Documents\Unreal Projects\LyraStarterGame*`) to auto-populate `game.contentSourcePath` in the setup wizard or suggest it during `ludus init`.
- [ ] **Server map validation** — Verify the configured `serverMap` exists in the project's cooked content or source assets before starting a multi-hour cook, instead of failing late in the pipeline.

## Build UX

Improving the experience during long build operations.

- [ ] **Progress indicators** — Periodically tail the UBA log during builds and print "X/Y actions (Z%)" summaries. Even without a progress bar, periodic status updates are far better than hours of silence.
- [ ] **Resume / incremental builds** — Detect partial builds (e.g. from a previous OOM or crash) and offer to resume rather than restart from scratch. UBT already supports incremental builds; Ludus should surface this.
- [ ] **Build config guidance** — Help users choose between Shipping (smaller binaries, optimized, no debug symbols) vs Development (larger, debuggable, faster iteration) with clear tradeoffs explained in CLI output and docs.

## Deploy UX

Smoothing out the deployment and testing workflow.

- [ ] **Cost estimate before deploy** — Before creating a fleet, show the estimated hourly cost for the selected instance type and region. Prevents bill shock for new users.
- [ ] **Auto-session (`--with-session`)** — `ludus deploy ec2 --with-session` that creates a game session immediately after the fleet goes active, saving a manual step.
- [ ] **Batch destroy** — `ludus deploy destroy --all` that reads all versioned state files (`state-ue54.json`, etc.) and tears down all fleets in one command.
- [ ] **Instance type guidance** — Recommend instance types based on game characteristics (CPU-bound vs memory-bound) and provide cost/performance comparisons.

## Diagnostics / Error Handling

Better observability and self-service troubleshooting.

- [ ] **`ludus doctor` command** — Comprehensive diagnostic tool (beyond `ludus status`) that checks for: stale DLLs, wrong toolchain version in env vs registry, disk space for upcoming builds, partial/corrupted build state, AWS credential expiry, common misconfigurations.
- [ ] **Guided error messages** — Every failure should tell the user exactly what to do next, not just what went wrong. Contextual fix suggestions based on exit codes, log patterns, and known issues per UE version.

## Multi-Version UX

Better support for testing across multiple UE versions.

- [ ] **`ludus config set` command** — Quick config switching from the CLI (`ludus config set engine.version 5.7.3`, `ludus config set gamelift.fleetName ludus-fleet-ue57`) instead of manually editing `ludus.yaml`.
- [ ] **State profiles** — Current single `state.json` is fragile for multi-version workflows. Support named state profiles or version-tagged state files natively, so switching between UE versions doesn't require manual state backup/restore.

## Features

Larger feature additions from the project roadmap.

- [ ] **ARM/Graviton support** — EC2 fleet deployment on Graviton instances for 20-30% cost savings. GameLift has supported Graviton since late 2023 with 130+ ARM instance types (c7g, c8g, m7g, r7g, etc.). UE5 already supports `LinuxArm64Server` as a build target. Implementation needs: ARM64 cross-compile toolchain for Windows (Epic ships `LinuxArm64` platform support but no official Windows→ARM cross-compile installer), `-platform=LinuxArm64` in BuildCookRun args, ARM-compatible Game Server Wrapper build, and `--arch arm64` flag in `ludus deploy ec2`.
- [ ] **BuildGraph XML generation** — `ludus buildgraph` command that generates BuildGraph XML validated against the UE schema. Outputs a ready-to-use XML file that UET, Horde, or other build orchestration tools can consume. An addition to the existing linear pipeline, not a replacement.
- [ ] **Studio infrastructure provisioning** — Potentially a separate project that provisions game studio infrastructure on AWS (Perforce, CI/CD build farms, derived data cache, virtual workstations) as composable modules that integrate with Ludus. Decision point: integrate with AWS's [cloud-game-development-toolkit](https://github.com/aws-games/cloud-game-development-toolkit), wrap it, or build from scratch.
