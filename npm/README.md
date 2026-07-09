# ludus-cli

**The fastest way to build, cook, and deploy Unreal Engine 5 dedicated servers.**

One command. Multiple backends. Production-ready GameLift, EC2, or binary output — with full AI agent integration via MCP.

## Install

```bash
npm install -g ludus-cli
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

## What it does

`ludus run` orchestrates the complete UE5 dedicated server pipeline:

1. **Prerequisite validation** — OS, engine source, game content, Docker, AWS CLI, disk space, RAM
2. **Engine build** — UE5 source compilation (Setup.sh, project files, make)
3. **Game server build** — Dedicated server packaging via RunUAT BuildCookRun
4. **Container build** — Dockerfile generation and Docker image build
5. **ECR push** — Docker image push to Amazon ECR
6. **GameLift deploy** — Container fleet creation with IAM roles and polling

Supports **UE 5.4 through 5.8**, with automatic toolchain resolution and cross-compilation.

## Deployment targets

| Target | Command | Docker required? | ARM64 (Graviton) |
|--------|---------|:---:|:---:|
| GameLift Containers | `ludus deploy fleet` | Yes | Yes |
| CloudFormation Stack | `ludus deploy stack` | Yes | Yes |
| GameLift Managed EC2 | `ludus deploy ec2` | No | Yes |
| GameLift Anywhere | `ludus deploy anywhere` | No | No |
| Binary export | `ludus deploy binary` | No | Yes |

**GameLift Anywhere** is ideal for local development — register your machine with GameLift and create fleets in seconds instead of minutes. No Docker build or ECR push needed.

## Build backends

Choose how engine and game builds execute:

| Backend | Description |
|---------|-------------|
| `native` (default) | Build directly on the host |
| `docker` | Build inside Docker containers — portable, reproducible |
| `podman` | Docker alternative — recommended on Windows for large engine images |
| `wsl2` | Native Linux I/O on Windows — 3-10× faster with `--wsl-native` ext4 sync |

## Key features

### Zen DDC — up to 59% faster cooks
Persistent shader and asset cache that survives container lifecycles. Enabled by default for UE 5.4+. A full Lyra cook drops from 37 minutes to 19 on a warm cache.

### ARM64 / Graviton support
Cross-compile from any platform to ARM64 and deploy to Graviton instances — 20-30% cheaper than x86. The `--arch arm64` flag threads through the entire pipeline, and Ludus auto-selects Graviton instance types for fleet deployment.

### Build caching
Input-hash based cache (`.ludus/cache.json`) skips unchanged stages automatically. Change one config value and only the affected stages rebuild.

### Build observability
Per-run build logs (`.ludus/logs/`) and optional OpenTelemetry (OTLP) trace export — one span per pipeline stage. Integrate with Grafana, Jaeger, or Tempo to see exactly where build time goes.

### Privacy — AWS Account ID masking
Your AWS account ID is masked in all terminal output by default (ECR URLs, ARNs, etc.) for safer screen sharing and recordings. Override with `--show-account-id` when needed.

### Comprehensive diagnostics
`ludus doctor` goes deeper than `init` — stale DLLs, toolchain mismatches, disk pressure, partial build state, AWS credential expiry, cache integrity, and Dockerfile linting (hadolint + trivy).

### Resource management
All AWS resources are tagged with `ManagedBy: ludus`. `ludus deploy destroy` tears down fleets safely, preserving durable ECR images and S3 buckets. Use `--purge` for a full wipe.

## AI Agent Integration (MCP)

Ludus ships with a built-in [Model Context Protocol](https://modelcontextprotocol.io/) server exposing **26 tools**. Any MCP-compatible AI agent — Claude Code, Cursor, Kiro, VS Code Copilot — can orchestrate the full pipeline programmatically.

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

## Documentation

Full documentation, configuration reference, and prerequisites: [github.com/jpvelasco/ludus](https://github.com/jpvelasco/ludus)

## License

Ludus is released under the **MIT License** (see [LICENSE](https://github.com/jpvelasco/ludus/blob/main/LICENSE) for full text).

All third-party dependencies are also MIT or Apache 2.0 licensed.

**Unreal Engine 5 usage note**:
This tool does **not** include or redistribute any UE5 source code or binaries.
You must obtain UE5 source code directly from Epic Games via GitHub (requires a valid Epic developer account). Ludus only orchestrates your legally obtained engine source and builds — all resulting engine images, game servers, and deployments are governed by Epic's EULA, which allows private use and modification but prohibits public distribution of built engine binaries.
