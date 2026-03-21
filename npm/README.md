# ludus-cli

CLI tool that automates the end-to-end pipeline for deploying Unreal Engine 5 dedicated servers to AWS GameLift.

Ludus handles the entire workflow that would otherwise require dozens of manual steps across multiple tools: UE5 source builds, game server compilation, Docker containerization, ECR push, and GameLift fleet deployment.

## Install

```bash
npm install -g ludus-cli
```

Or run directly:

```bash
npx ludus-cli --help
```

## What it does

```
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
