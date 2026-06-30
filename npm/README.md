# ludus-cli

CLI tool that automates the end-to-end pipeline for deploying Unreal Engine 5 dedicated servers to AWS GameLift.

Ludus handles the entire workflow that would otherwise require dozens of manual steps across multiple tools: UE5 source builds, game server compilation, Docker containerization, ECR push, and GameLift fleet deployment.

## Install

```bash
npm install -g ludus-cli
```

Upgrade to the latest version the same way:

```bash
npm install -g ludus-cli@latest
```

Or run directly:

```bash
npx ludus-cli --help
```

The package downloads a small prebuilt binary on install. If your environment
blocks install scripts (e.g. `--ignore-scripts`, pnpm, locked-down CI), `ludus`
fetches the matching binary on first run instead — no extra steps. To manage the
binary yourself (air-gapped setups), set `LUDUS_SKIP_AUTO_DOWNLOAD=1` and place
the `ludus` binary under the package's `bin/` directory.

### "allow-scripts" warning during install

If npm's `allow-scripts` policy is enabled in your environment, you may see:

```text
npm warn allow-scripts   ludus-cli@x.y.z (postinstall: node install.js)
```

This is **harmless** — the install still succeeds. `postinstall` is only the
binary downloader, and `ludus` self-heals by fetching the binary on first run if
the script was blocked. So you can ignore the warning and just run `ludus`. To
silence it and let the download run at install time, allow-list the package with
the command npm prints (e.g. `npm config set allow-scripts=ludus-cli --location=user`).

## What it does

```bash
ludus run --verbose
```

This single command orchestrates six stages:

1. **Prerequisite validation** — OS, engine source, game content, Docker, AWS CLI, disk space, RAM
2. **Engine build** — UE5 source compilation
3. **Game server build** — Dedicated server packaging via RunUAT
4. **Container build** — Dockerfile generation and Docker image build
5. **ECR push** — Docker image push to Amazon ECR
6. **GameLift deploy** — Container fleet creation with IAM roles and polling

## Deployment targets

| Target | Command | Docker required? | ARM64 (Graviton) |
|--------|---------|:---:|:---:|
| GameLift Containers | `ludus deploy fleet` | Yes | Yes |
| CloudFormation Stack | `ludus deploy stack` | Yes | Yes |
| GameLift Managed EC2 | `ludus deploy ec2` | No | Yes |
| GameLift Anywhere | `ludus deploy anywhere` | No | No |
| Binary export | `ludus deploy binary` | No | Yes |

## AI Agent Integration (MCP)

Ludus includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/) server exposing 21 tools. Any MCP-compatible AI agent can orchestrate the full pipeline programmatically.

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

## Documentation

Full documentation, configuration reference, and prerequisites: [github.com/jpvelasco/ludus](https://github.com/jpvelasco/ludus)

## License

Ludus is released under the **MIT License** (see [LICENSE](https://github.com/jpvelasco/ludus/blob/main/LICENSE) for full text).

All third-party dependencies are also MIT or Apache 2.0 licensed.

**Unreal Engine 5 usage note**:
This tool does **not** include or redistribute any UE5 source code or binaries.
You must obtain UE5 source code directly from Epic Games via GitHub (requires a valid Epic developer account). Ludus only orchestrates your legally obtained engine source and builds — all resulting engine images, game servers, and deployments are governed by Epic's EULA, which allows private use and modification but prohibits public distribution of built engine binaries.
