# Architecture

High-level overview of how Ludus is structured and how data flows through the pipeline.

## Pipeline

Ludus orchestrates six stages, each independently runnable or chained via `ludus run`:

```
ludus.yaml
    |
    v
 1. init ........... validate prerequisites (OS, engine, toolchain, content)
    |
    v
 2. engine build ... compile UE5 from source
    |
    v
 3. game build ..... build dedicated server (+ optional client) via RunUAT
    |
    v
 4. container build  generate Dockerfile, build image  [skipped by ec2, anywhere, binary]
    |
    v
 5. deploy ......... push to target
    |               ├── gamelift ... container fleet on GameLift
    |               ├── stack ...... CloudFormation (atomic, rollback)
    |               ├── ec2 ........ Managed EC2 (no Docker)
    |               ├── anywhere ... local dev via GameLift Anywhere
    |               └── binary ..... export files to disk
    v
 6. connect ........ create game session + launch client
```

Not all stages run for every target. The pipeline checks `target.Capabilities()` and skips
stages that don't apply (e.g. `anywhere` and `ec2` skip the container build entirely).

## Project Layout

```
ludus/
├── main.go                     # Entry point -> cmd/root
├── cmd/                        # CLI commands (Cobra)
│   ├── root/                   # Root command, subcommand registration, init command
│   ├── globals/                # Shared mutable state (Cfg, Verbose, DryRun) + target factory
│   ├── setup/                  # Interactive first-run wizard
│   ├── engine/                 # ludus engine build|setup|push
│   ├── game/                   # ludus game build|client
│   ├── container/              # ludus container build|push
│   ├── deploy/                 # ludus deploy fleet|stack|ec2|anywhere|session|destroy
│   ├── connect/                # ludus connect (platform-specific client launch)
│   ├── doctor/                 # ludus doctor (diagnostics)
│   ├── status/                 # ludus status (pipeline stage checks)
│   ├── pipeline/               # ludus run (full pipeline orchestration)
│   ├── configcmd/              # ludus config set|get
│   ├── ci/                     # ludus ci init|runner
│   ├── buildgraph/             # ludus buildgraph (XML generation)
│   ├── ddc/                    # ludus ddc status|clean|prune|warmup
│   ├── logs/                   # ludus logs list|path|tail (per-run build logs)
│   └── mcp/                    # MCP server (26 tools for AI agents)
├── internal/                   # Business logic (unexported)
│   ├── config/                 # Config loading, arch helpers
│   ├── runner/                 # Shell execution abstraction
│   ├── retry/                  # Retry with exponential backoff for Docker/AWS calls
│   ├── engine/                 # UE5 engine build orchestration
│   ├── game/                   # Server + client build via RunUAT
│   ├── cleanup/                # AWS resource cleanup helpers
│   ├── container/              # Dockerfile generation, Docker build, ECR push
│   ├── ddc/                    # Derived Data Cache management (persistent shader/asset cache)
│   ├── deploy/                 # Target interface definition
│   ├── gamelift/               # GameLift container fleet deployer
│   ├── glsession/              # GameLift game session management
│   ├── inventory/              # AWS resource inventory and discovery
│   ├── stack/                  # CloudFormation stack deployer
│   ├── ec2fleet/               # Managed EC2 fleet deployer
│   ├── ecr/                    # ECR repository and image operations
│   ├── anywhere/               # GameLift Anywhere (local) deployer
│   ├── binary/                 # Binary file exporter
│   ├── dockerbuild/            # Engine/game builds inside Docker
│   ├── wrapper/                # GameLift Game Server Wrapper (Go binary)
│   ├── toolchain/              # Cross-compile toolchain version mapping
│   ├── prereq/                 # Prerequisite validation (platform-specific)
│   ├── buildgraph/             # BuildGraph XML schema + generator
│   ├── pricing/                # Instance type defaults + cost estimation
│   ├── diagnose/               # Error pattern matching + hints
│   ├── dflint/                 # Dockerfile security linting
│   ├── state/                  # Pipeline state persistence (.ludus/state.json)
│   ├── cache/                  # Build cache (input hashing, skip logic)
│   ├── status/                 # Pipeline status checks
│   ├── tags/                   # AWS resource tagging
│   ├── awsutil/                # Shared AWS SDK helpers (credentials, region)
│   ├── ci/                     # GitHub Actions workflow + runner management
│   ├── progress/               # Elapsed-time progress indicators
│   ├── buildlog/               # Per-run build log persistence (.ludus/logs/)
│   ├── tracing/                # Optional OpenTelemetry (OTLP) trace export
│   ├── version/                # Build version injection
│   └── wsl/                    # WSL2 detection and path translation
└── npm/                        # npm wrapper for `npx ludus-cli mcp`
```

## Key Design Decisions

### Deploy Target Interface

All deploy backends implement `deploy.Target`:

```go
type Target interface {
    Deploy(ctx context.Context) (*Result, error)
    Destroy(ctx context.Context) error
    CreateSession(ctx context.Context) (*SessionInfo, error)
    Status(ctx context.Context) (*StatusInfo, error)
    Capabilities() Capabilities
}
```

`Capabilities()` returns which pipeline stages the target needs (e.g. `NeedsContainer`,
`NeedsECRPush`). The pipeline uses this to skip unnecessary work. New targets implement
the interface and get wired into the factory in `cmd/globals/resolve.go`.

### Runner Abstraction

All shell execution goes through `runner.Runner`, never raw `exec.Command`. The runner
handles `--verbose` output (printing commands as `+ cmd args`), `--dry-run` (print without
executing), and consistent error wrapping. This is non-negotiable — it's how the CLI stays
predictable across 30+ different external tool invocations.

### MCP Server

The MCP server (`cmd/mcp/`) exposes the same logic as the CLI through 26 JSON-RPC tools.
Stdout is redirected to stderr (MCP uses stdout for the protocol), and `withCapture()`
collects output per tool call. Long-running operations (engine/game builds) have async
variants that return a build ID immediately — agents poll with `ludus_build_status`.

### Cross-Architecture Support

The `--arch` flag threads through the entire pipeline:

```
game build --arch arm64
  -> RunUAT with LinuxArm64 platform
  -> container build --platform linux/arm64
  -> deploy with Graviton instance type (auto-detected)
```

Architecture mismatches are caught automatically — if you build arm64 but your fleet config
says `c6i.large` (x86), Ludus switches to `c7g.large` (Graviton) and tells you.

On macOS with container backends (`--backend docker`/`podman`), engine builds are forced to `linux/amd64` (QEMU user-mode emulation required; Epic only provides an x86_64 Linux toolchain). Game builds with `--arch arm64` (Graviton) cross-compile inside the emulated amd64 environment; the resulting engine image stays amd64 (even for later arm64 game containers). Pre-built amd64 engine images (from Linux/CI) are recommended to avoid emulation cost. The `--arch` flag and mismatch handling apply as above.

### DDC Backend (Zen vs legacy FileSystem)

Unreal Engine uses the **Zen Store** as its default local DDC backend from **UE 5.4 onward** (the
legacy FileSystem DDC is delete-only since 5.4). Ludus supports UE 5.4–5.8, so `ddc.mode` defaults
to `zen` unconditionally — there is no version gate. (An earlier assumption that ZenStore applied
only to UE 5.6+ was incorrect; if you find a "5.6+" reference to DDC, it is a bug.)

`zen` means different concrete things per build path, because only containers have an ephemeral
filesystem that loses the cache:

```
mode: zen
  container (docker/podman)  -> bind-mount host ~/.ludus/zen at the container's
                                Zen data path (ddc.ZenContainerPath) so it survives --rm
                                [internal/dockerbuild/game.go: zenDDCArgs]
  native / wsl2 (binary)     -> no-op: UE autolaunches its Zen Store into the real
                                home dir, which already persists across runs
                                [internal/game/ddc.go: setupDDC; internal/wsl/game.go]

mode: local (deprecated)
  container                  -> mount host ~/.ludus/ddc at /ddc + UE-LocalDataCachePath
  native / wsl2              -> set UE-LocalDataCachePath to the host path
```

Consequence: `ludus ddc clean|prune|status` manage the Ludus-owned directories (the Zen mount for
containers, the FileSystem cache for `local`). They do **not** manage the native/WSL2 Zen location
(UE owns it in the user's home). Docker and Podman are identical here — the backend string only
selects the executable in `ContainerCLI`.

A deprecation warning for `mode: local` is centralized in `ddc.LocalModeDeprecationWarning` and
surfaced from config load (`cmd/root`), `ludus doctor` (`checkDDCMode`), and `ludus ddc status`.

### Configuration Flow

```
ludus.yaml -> Viper -> config.Config struct
                           |
                    CLI flags override
                           |
                    MCP tool params override
                           |
                    cmd/ handlers consume
```

The config struct is loaded once in `PersistentPreRunE` and stored in `globals.Cfg`.
CLI flags and MCP parameters override specific fields before passing to `internal/` logic.

## State and Caching

Ludus persists two files in `.ludus/`:

- **`state.json`** — Fleet IDs, session IPs, ECR URIs, build paths. Written by deploy/build
  commands, read by downstream commands (e.g. `connect` reads session IP from state).
- **`cache.json`** — Input hashes per pipeline stage. If inputs haven't changed, the stage
  is skipped. `--no-cache` forces a rebuild.

Profiles (`--profile <name>`) isolate both: config reads from `ludus-<name>.yaml`,
state writes to `.ludus/profiles/<name>.json`.

## Build Observability

Build output is observable in two complementary ways, both opt-out/opt-in rather than always-on noise:

- **On-disk logs** (`internal/buildlog`) — CLI runners tee stdout/stderr to a per-run file
  under `.ludus/logs/`, opened lazily on first use and pruned to `observability.logs.retainRuns`.
  Query with `ludus logs list|path|tail`; disable with `--no-logs`. Dry-run is never logged.
- **Distributed traces** (`internal/tracing`) — optional OpenTelemetry (OTLP) export emitting one
  span per pipeline stage under a `ludus.run` root span. A no-op with zero overhead unless
  `observability.otlp.enabled` (or standard `OTEL_*` env vars) is set; initialized in root
  `PersistentPreRunE` and flushed on exit.
