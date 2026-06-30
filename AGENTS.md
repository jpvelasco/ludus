# Ludus Development Guide

This guide provides essential context for AI agents working with Ludus, a CLI tool that automates the end-to-end pipeline for deploying Unreal Engine 5 dedicated servers to AWS GameLift.

## Build, Test & Lint

```bash
# Build (Windows)
go build -o ludus.exe -v .

# Build (Linux/macOS)
go build -o ludus -v .

# Lint (golangci-lint v2 required)
golangci-lint run ./...

# Test (all packages)
go test ./...

# Test with race detector (Linux/macOS; requires CGO)
go test -race ./...

# Test (single package)
go test -v ./internal/toolchain

# Test (single test)
go test -v -run TestParseBuildVersion ./internal/toolchain

# Static analysis
go vet ./...

# Module cleanup
go mod tidy

# Pre-commit hooks check
.hooks/pre-commit
```

## Key Project Structure

- `main.go` → `cmd/root/root.go` → subcommand packages in `cmd/`
- `cmd/globals/globals.go` — shared mutable state (`Cfg`, `Verbose`, `DryRun`, `JSONOutput`, `Profile`)
- `cmd/mcp/` — MCP server with 26 tools for AI orchestration
- `internal/` — all business logic (unexported); most files stay close to one primary type, but some packages are deliberately split across sibling files by concern when complexity gets too high
- Platform-specific files: `_windows.go` / `_unix.go` with `//go:build` tags

## Critical Features to Understand

### Deployment & Build Backends
- `--backend native` (default, builds on host)
- `--backend docker` (builds engine container images, needs Docker)
- `--backend podman` (Docker alternative, on Windows or Linux)
- `--backend wsl2` (native Linux I/O on Windows)

### Architecture Support
- `--arch amd64` (default)
- `--arch arm64` (Graviton instances for AWS)
- Cross-compilation from Windows to Linux binaries

### DDC (Derived Data Cache) Modes
- `--ddc zen` (default, persistent UE Zen Store cache)
- `--ddc local` (legacy FileSystem cache, deprecated — recommend `zen`)
- `--ddc none` (disabled)

### Coverage

- Coverage is uploaded to Codecov from the ubuntu test leg via OIDC.
- Patch coverage is enforced at 80% in `codecov.yml`; new or changed lines under that threshold post a failing `codecov/patch` status.
- It is a soft block, not a required check, so genuinely E2E-only code can still merge with judgment.

## Development Environment

- Go 1.25.11 required (see `go.mod`; CI follows it)
- Linux or Windows with Docker/Podman for container builds
- AWS CLI v2 configured with credentials
- UE5 source with Lyra game assets (must be downloaded manually)
- 16+ GB RAM recommended (UE5 compilation is memory-hungry)
- 300+ GB disk space for engine builds

## Important Operational Details

- Run external processes through `runner.Runner` (not raw `exec.Command`)
- Use `internal/retry` for AWS/Docker operations (default retry strategy)
- Use `awsutil.Poll` for AWS wait loops
- Use `internal/awsenv` (NewResolver + Requirements + ImageURI/RegistryURI) for all account/region resolution and ECR URI building; centralized to address per-command duplication (see #367)
- All CLI commands support `--verbose`, `--dry-run`, JSON output
- Commands support `--profile` (creates `ludus-<name>.yaml`)
- Config loaded from `ludus.yaml` via Viper (override via `--config`)
- Build logs go to `.ludus/logs/` (project-local)
- Cache in `.ludus/cache.json` (skip unchanged stages automatically)
- State in `.ludus/state.json` (fleet IDs, ECR URIs, session data)

## Communication Model

Ludus uses the [Model Context Protocol](https://modelcontextprotocol.io/).
The MCP server is started with `ludus mcp` and exposes 26 tools for AI orchestration.

## Command Execution Patterns

- Commands produce output via `fmt.Printf` (status) and `fmt.Fprintln(os.Stderr)` (warnings)
- All test commands expect the working directory to be the project root
- Commands with long-running operations provide `*_start` variants for async operations
- Environment variables and CLI flags are merged with config precedence:
  `ludus.yaml` → `--flag` → `mcp-parameter`

## Testing Constraints

- All tests use Go standard library only.
- Prefer table-driven tests with `tt` as the loop variable, and keep tests in the same package when they need unexported symbols.
- Use `t.TempDir()` for temporary directories, `t.Setenv()` for environment variable overrides, and `t.Chdir()` for working directory changes.
- AWS/Docker/`wsl.exe`/subprocess-bound code (`gamelift`, `ec2fleet`, `stack`, `wrapper`, `wsl`, and most deploy logic) is E2E-covered; unit tests there should cover only the pure surface such as adapter `Name`/`Capabilities`, argument assembly, and parameter parsing.
- Keep each test function under cyclomatic complexity 8. Codacy's Lizard check counts test files and fails PRs otherwise; convert flat assertion chains to map/table loops and extract `t.Run` bodies into named helpers. Verify with `go run github.com/fzipp/gocyclo/cmd/gocyclo@latest -over 8 <file>` printing nothing.
- Read mutex-guarded struct fields under the same lock in tests. CI runs `-race` on ubuntu and macOS, so unlocked reads of fields that a goroutine writes under a lock can fail CI even if they pass locally; a channel signal is not a happens-before edge for a lock-protected write.
- Unit test files stay co-located with source files.
