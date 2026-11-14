# ludus-cli

**A fast, single-command way to build, cook, and deploy Unreal Engine 5 dedicated servers.**

One command. Multiple backends. Production-ready GameLift, EC2, or binary output — with full AI agent integration via MCP.

This is the UE5 / GameLift CLI. It is unrelated to other products named Ludus.

## Install

The registry name is **`ludus-cli`**. After a global install the on-PATH command is **`ludus`** (this package's `bin` entry), which is what Quickstart and the rest of this README invoke. `npx ludus-cli …` runs that same binary without a global install.

Published versions match [GitHub Releases](https://github.com/jpvelasco/ludus/releases) — treat the tagged `v*` releases as the version source of truth.

```bash
npm install -g ludus-cli
ludus --help
```

Upgrade to the latest version the same way:

```bash
npm install -g ludus-cli@latest
```

Or run directly without installing:

```bash
npx ludus-cli --help
```

The package downloads the matching prebuilt binary on install. If your environment
blocks install scripts (e.g. `--ignore-scripts`, pnpm, locked-down CI), `ludus`
self-heals by fetching the binary on first run — no extra steps. Air-gapped? Set
`LUDUS_SKIP_AUTO_DOWNLOAD=1` and drop the `ludus` binary under the package's `bin/` directory.

### "allow-scripts" warning during install

If npm's `allow-scripts` policy is enabled, you may see:

```text
npm warn allow-scripts   ludus-cli@x.y.z (postinstall: node install.js)
```

This is **harmless** — the install still succeeds. The binary downloader runs on
first use if the script was blocked. To silence the warning, allow-list the package:

```bash
npm config set allow-scripts=ludus-cli --location=user
```

## Quickstart

```bash
# Install and configure
npm install -g ludus-cli
ludus setup

# Validate your environment
ludus init --verbose

# Run the full pipeline — one command
ludus run --verbose
```

### Prerequisites

- UE **5.4–5.8 source**, obtained through your Epic Games account. Launcher
  engine builds cannot produce dedicated server targets. Use Git to manage the
  engine source checkout.
- Your game's assets. For Lyra, download the matching **Lyra Starter Game**
  separately from Epic/Fab; GitHub engine source does not include them. Set
  `game.contentSourcePath` to the downloaded project and run `ludus init --fix`
  to overlay its content. Lyra validation follows `game.projectPath` when set
  and checks both `Content/DefaultGameData.uasset` and GameFeature plugin content.
- Docker or Podman for container builds, and AWS CLI v2 with credentials for
  AWS targets. `ludus container build` also needs **Go >= 1.20 and Make** on PATH
  to compile the server wrapper, even when Ludus is installed through npm.
- At least **16 GB RAM** and **300 GB free disk** for native builds.
  Container builds need more space: **1 TB minimum, 2 TB recommended**.
  `ludus init` checks a 300 GB disk floor; passing that check does not establish
  enough space for container builds.
- Windows or Linux for native builds; macOS requires Docker or Podman.

See the [full prerequisites](https://github.com/jpvelasco/ludus#prerequisites)
for engine access, asset download instructions, and AWS permissions.

## What it does

`ludus run` orchestrates the complete UE5 dedicated server pipeline:

1. **Prerequisite validation** — OS, engine source, game content, Docker, AWS CLI, disk space, RAM
2. **Engine build** — UE5 source compilation (Setup.sh, project files, make)
3. **Game server build** — Dedicated server packaging via RunUAT BuildCookRun
4. **Container build** — Dockerfile generation and Docker image build
5. **ECR push** — Docker image push to Amazon ECR
6. **GameLift deploy** — Container fleet creation with IAM roles and polling

Supports **UE 5.4 through 5.8**, with automatic toolchain resolution and cross-compilation.
The selected deployment target determines which stages are needed: managed EC2
and Anywhere skip container builds and ECR push.

## Deployment targets

| Target | Command | Docker required? | ARM64 (Graviton) |
|--------|---------|:---:|:---:|
| GameLift Containers | `ludus deploy fleet` | Yes | Yes |
| CloudFormation Stack | `ludus deploy stack` | Yes | Yes |
| GameLift Managed EC2 | `ludus deploy ec2` | No | Yes |
| GameLift Anywhere | `ludus deploy anywhere` | No | No |
| Binary export | `ludus deploy binary` | No | Yes |

**GameLift Anywhere** is ideal for local development — register your machine with GameLift and create fleets in seconds instead of minutes. No Docker build or ECR push needed. Custom `locationName` values must start with `custom-` (Ludus prefixes the name if you omit it) and persist the normalized name in state. After a failed deploy, run `ludus deploy destroy --target anywhere` then `ludus status`. Full runbook: [GameLift Anywhere](https://github.com/jpvelasco/ludus#gamelift-anywhere-local-development).

### Configuration and resource ownership

`ludus setup` creates `ludus.yaml`. Set `engine.sourcePath`, `game.projectPath`
(the full `.uproject` filename), and your AWS region before building.
`game.projectName` supplies default target names; packaged paths follow the
`.uproject` filename. Set `game.serverTarget` explicitly if it differs from
`<projectName>Server`.

`ManagedBy` defaults to `ludus` and `Project` defaults to `game.projectName`.
Overrides under `aws.tags` are honored by role ownership checks, resource
discovery, and newly created ECR repositories. Pushing images leaves existing
repository tags unchanged.

Direct container and managed-EC2 fleet roles are reused or deleted only when
their `ManagedBy`, `Project`, and fleet attribution tags match the current
configuration. Tag keys are compared without case sensitivity; values must
match exactly. CloudFormation-managed roles and older roles without attribution
are retained. Choose a unique `gamelift.iam.roleName` for an unrelated role name
collision. Tags establish attribution; they do not prove exclusive use.

For direct container fleets, `gamelift.ownershipTagKey` changes the fleet,
container-group, and IAM role attribution key (default `ludus:fleet-name`).
Existing tags are retained, with reads falling back to the legacy key when the
configured key is absent. Conflicting IAM ownership values prevent role
mutation. This setting does not change EC2 or CloudFormation tag keys.
Keep the original configuration for teardown after changing deployment names
or ownership tags.

GameLift container fleet roles require `gamelift.iam.path` to be empty or `/`.
`ludus deploy fleet --dry-run` shows the resolved role name, path, permissions
boundary, tags, and containers per instance. `gamelift.maxConcurrentSessions`
sets game-server container groups per instance; available session capacity also
depends on fleet instances and game server processes.

Full [configuration reference](https://github.com/jpvelasco/ludus#configuration)
and [AWS permissions and ownership rules](https://github.com/jpvelasco/ludus#aws-setup).

### Preview and tear down deployments

```bash
# Preview the active target without AWS calls or local changes
ludus deploy destroy --dry-run

# Preview all target types and durable artifacts in the active profile as JSON
ludus deploy destroy --all-targets --purge --dry-run --json

# Remove the active target's deployment; preserve durable artifacts
ludus deploy destroy
```

The offline preview lists recorded IDs, configured names, local paths, and
conditions that require live checks. It does not resolve AWS credentials, start
subprocesses, prompt, or change files. Application telemetry follows its usual
settings. Resource existence, IAM ownership, and shared container-group
references remain unverified until execution. Missing IDs appear as discovery
selectors or unresolved conditions.

`--all-targets` selects every target type in the **active profile**.
`--purge` independently adds the configured server ECR repository (all images)
and configured/account-derived S3 build bucket, with a confirmation prompt.
`--yes` skips that prompt for automation. Purge does not select the engine ECR
repository or other profiles.

Default cleanup follows each target: fleets/groups/roles for containers,
CloudFormation stack deletion rules for `stack`, fleet/build/uploaded S3 object/
role for `ec2`, local server/compute/fleet/location for `anywhere`, or the output
directory for `binary`. Durable ECR repositories and S3 build buckets remain
unless purge is selected and can continue incurring storage costs.

```bash
# Preview first, then delete this profile's targets and named durable artifacts
ludus deploy destroy --all-targets --purge --dry-run
ludus deploy destroy --all-targets --purge
```

Transcript values are quoted and control characters escaped. Terminal account
IDs follow the privacy setting; JSON and MCP preserve original identifiers.
Full [teardown runbook](https://github.com/jpvelasco/ludus#tearing-down-ludus-deploy-destroy).

## Build backends

Choose how engine and game builds execute:

| Backend | Description |
|---------|-------------|
| `native` (default) | Build directly on the host |
| `docker` | Build inside Docker containers — portable, reproducible |
| `podman` | Docker alternative — recommended on Windows for large engine images |
| `wsl2` | Native Linux I/O on Windows — 3-10× faster with `--wsl-native` ext4 sync |

On macOS, use `--backend docker` or `--backend podman`. Engine images always
target `linux/amd64`; Apple Silicon runs those builds through QEMU. To skip
engine compilation under emulation, configure a pre-built image with
`engine.dockerImage`. `--arch arm64` still cross-compiles Graviton game servers
inside that environment. See [macOS support](https://github.com/jpvelasco/ludus#macos-support).

## Key features

### Zen DDC — up to 59% faster cooks
Persistent shader and asset cache that survives container lifecycles. Enabled
by default for UE 5.4+. In the measured Lyra/UE 5.7.4 WSL2 native run, the cook
phase dropped from 22 minutes to 9; the full BuildCookRun dropped from 37 minutes
to 19. See the [benchmark and setup](https://github.com/jpvelasco/ludus#ddc-performance--up-to-59-faster-cooks-on-wsl2-native).

### ARM64 / Graviton support
Cross-compile from any platform to ARM64 and deploy to Graviton instances — 20-30% cheaper than x86. The `--arch arm64` flag threads through the entire pipeline, and Ludus auto-selects Graviton instance types for fleet deployment.

### Build caching
Input-hash based cache (`.ludus/cache.json`) skips unchanged stages automatically. Change one config value and only the affected stages rebuild.

### Build observability
Per-run build logs (`.ludus/logs/`) and optional OpenTelemetry (OTLP) trace export — one span per pipeline stage. Integrate with Grafana, Jaeger, or Tempo to see exactly where build time goes. **Telemetry: none to the maintainer.** OTLP export is opt-in and sends spans only to the collector you configure (or the one your `OTEL_*` environment points to); `ludus doctor` reports when it is active.

### Privacy — AWS Account ID masking
Your AWS account ID is masked in human-readable terminal output by default
(ECR URLs, ARNs, destroy previews, etc.) for screen sharing and recordings.
Override with `--show-account-id` when needed. JSON and MCP responses retain
raw identifiers for machine use.

### Comprehensive diagnostics
`ludus doctor` goes deeper than `init` — stale DLLs, toolchain mismatches, disk pressure, partial build state, AWS credential expiry, cache integrity, and Dockerfile linting (hadolint + trivy).
`ludus status` checks the local engine image for Docker/Podman backends and
reports a configured pre-built image separately; native/WSL2 checks use engine
build artifacts.

### Resource management
`ludus resources` discovers resources using the effective `ManagedBy` and
`Project` tags. It reports incomplete results when tag discovery is denied and
still scans configured ECR names and S3 build buckets; configured ECR names have
unverified ownership. IAM users and roles are outside this inventory.
Use the [preview and teardown workflow](#preview-and-tear-down-deployments) to
inspect cleanup scope.

## AI Agent Integration (MCP)

Ludus ships with a built-in [Model Context Protocol](https://modelcontextprotocol.io/) server exposing **26 tools**. Any MCP-compatible AI agent — Claude Code, Cursor, Kiro, VS Code Copilot — can orchestrate the full pipeline programmatically.
`ludus mcp` uses **stdio by default**. Run it from the project directory with
the intended configuration and profile.

**Claude Code:**
```bash
claude mcp add ludus -- npx -y ludus-cli mcp
```

**Claude Desktop / Kiro / Cursor:**
```json
{
  "mcpServers": {
    "ludus": {
      "command": "npx",
      "args": ["-y", "ludus-cli", "mcp"]
    }
  }
}
```

**VS Code Copilot:**
```json
{
  "servers": {
    "ludus": {
      "command": "ludus",
      "args": ["mcp"]
    }
  }
}
```

Tools cover the full pipeline: init, engine build, game build, container build/push, all five deploy targets, game sessions, connections, DDC management, BuildGraph generation, and async build lifecycle (start, poll, cancel).

Use `ludus_engine_build_start`, `ludus_game_build_start`, or
`ludus_game_client_start` for long native/WSL2 builds, then poll or cancel through
`ludus_build_status`. Async container builds are not supported yet.
`ludus_deploy_destroy` returns the offline plan and shared transcript when
`dry_run` or the global `--dry-run` flag is set; `all_targets` and `purge`
select the same independent scopes as the CLI.

### Remote build hosts over Streamable HTTP

On a POSIX build host, create a private bearer-token file and start Ludus from
the project directory:

```bash
umask 077
openssl rand -hex 32 > "$HOME/.ludus-mcp-token"
ludus mcp --listen 127.0.0.1:8080 --token-file "$HOME/.ludus-mcp-token"
```

Forward host port `8080` to local port `8080` through SSM port forwarding or
another encrypted tunnel. Configure the client's Streamable HTTP endpoint as
`http://127.0.0.1:8080/mcp` and authorization header as
`Authorization: Bearer <token>`, storing the token in the client's secret storage.
The server does not print the token.

`--listen` selects HTTP unless `--transport` was explicitly set.
`--transport http` defaults to `127.0.0.1:8080`. HTTP always requires
`LUDUS_MCP_TOKEN` or `--token-file` (the file takes precedence); stdio needs no
token and rejects HTTP-only flags. Non-loopback listeners require
`--allow-remote`. HTTP provides no TLS, so use an encrypted tunnel or trusted
TLS proxy.

A valid token grants access to all tools, including creation and deletion,
under the server process's AWS credentials. Clients share its configuration,
profile, and async build manager. The HTTP transport is stateless; build IDs
remain available for polling across requests. Ctrl+C or SIGTERM stops accepting
connections and cancels active requests with a bounded shutdown wait.
Full [MCP setup and tool reference](https://github.com/jpvelasco/ludus#ai-agent-integration-mcp).

## Documentation

Full documentation, configuration reference, and prerequisites: [github.com/jpvelasco/ludus](https://github.com/jpvelasco/ludus)

## License

Ludus is released under the **MIT License** (see [LICENSE](https://github.com/jpvelasco/ludus/blob/main/LICENSE) for full text).

All third-party dependencies are also MIT or Apache 2.0 licensed.

**Unreal Engine 5 usage note**:
This tool does **not** include or redistribute any UE5 source code or binaries.
You must obtain UE5 source code directly from Epic Games via GitHub (requires a valid Epic developer account). Ludus only orchestrates your legally obtained engine source and builds — all resulting engine images, game servers, and deployments are governed by Epic's EULA, which allows private use and modification but prohibits public distribution of built engine binaries.
