# Coding Guidelines

For contributors and AI coding agents working in this repository.
See also [ARCHITECTURE.md](ARCHITECTURE.md) for the high-level design and module map.

## Build / Lint / Test

Go `1.25.10` is required (see `go.mod`).

```bash
# Build
go build -o ludus -v .            # Linux/macOS
go build -o ludus.exe -v .        # Windows

# Lint (golangci-lint v2 required)
golangci-lint run ./...

# Test
go test ./...                          # All tests
go test -race ./...                    # Race detector (Linux/macOS; requires CGO)
go test -v ./internal/toolchain        # Single package
go test -v -run TestParseBuildVersion ./internal/toolchain  # Single test

# Other
go vet ./...                      # Static analysis
go mod tidy                       # Clean up module dependencies
```

Pre-commit hooks (`.hooks/pre-commit`) run build, lint, and test.
Activate with `git config core.hooksPath .hooks`.

CI runs build and tests on Linux, Windows, and macOS, plus lint on Linux and
Windows. Linux tests also collect coverage; Linux and macOS tests use `-race`.

## Project Structure

- `main.go` — Entry point; calls `root.Execute()`.
- `cmd/` — Cobra command packages. Each exports `var Cmd = &cobra.Command{...}`,
  registered in `cmd/root/root.go`. Handler functions are named `run<Command>`.
  - `cmd/globals/` exports mutable global state (`Cfg`, `Verbose`, `JSONOutput`,
    `DryRun`, `Profile`, `DDCMode`) and deployment target resolution
    (`resolve.go`). It is not a command package. The `init` command lives in
    `cmd/root/init.go`.
  - `cmd/mcp/` — MCP server for AI agent orchestration (26 tools, stdio JSON-RPC).
  - `cmd/resources/` — AWS resource inventory for resources managed by Ludus.
- `internal/` — All business logic (unexported). One primary type per file.
  See [ARCHITECTURE.md](ARCHITECTURE.md) for the full package layout.
- Shared infrastructure includes `runner/` (subprocesses), `retry/` (network
  retries), `awsutil/` (AWS config/errors/polling), `state/` and `cache/`
  (persistent pipeline data), `inventory/` (AWS discovery), and `wsl/` (WSL2
  detection, paths, commands, and source synchronization).
- Config is loaded via Viper from `ludus.yaml`; `--profile <name>` prefers
  `ludus-<name>.yaml` and isolates persisted state.
- Platform-specific files use `_windows.go` / `_unix.go` suffixes with `//go:build` tags.

### Execution and External Services

- Run external processes through `runner.Runner`; do not call `exec.Command`
  directly. This preserves `--verbose`, `--dry-run`, environment overrides,
  output routing, and consistent error handling.
- Use `internal/retry` for retryable Docker/AWS operations. Use
  `retry.Default()` unless the operation needs explicit timing or attempt limits.
- Use `awsutil.Poll` / `PollWithOptions` for AWS wait loops and
  `awsutil.IsNotFound` / `IsConflict` for idempotent AWS error handling.
- Deploy backends implement `deploy.Target`; session-capable targets also
  implement `deploy.SessionManager`. Wire new targets through
  `cmd/globals/resolve.go` and expose them consistently in CLI, MCP, and status.

## Code Style

### Formatting and Imports

Enforced by `gofmt`. Two import groups separated by a blank line: (1) stdlib,
(2) everything else (third-party and project imports together, sorted alphabetically).

```go
import (
    "context"
    "fmt"

    "github.com/jpvelasco/ludus/internal/runner"
    "github.com/spf13/cobra"
)
```

Aliases only to resolve naming conflicts. Common patterns: AWS type packages
(`gltypes`, `cftypes`), cmd-vs-internal disambiguation (`engBuilder`, `gameBuilder`).

### Naming

- **Packages**: lowercase, single word. Multi-word concatenated (`dockerbuild`).
- **Files**: `snake_case.go`. Build-tagged: `checker_unix.go`, `process_windows.go`.
- **Acronyms**: Fully uppercase: `ID`, `URI`, `ARN`, `ECR`, `IAM`, `AWS`.
- **Structs**: PascalCase nouns — `Builder`, `Deployer`, `TargetAdapter`.
- **Options/Results**: `BuildOptions`, `BuildResult`, `DeployOptions`, `FleetStatus`.
- **Constants**: Unexported camelCase (`iamRoleName`), exported PascalCase (`WrapperRepo`).
- **Variables**: camelCase. Short for narrow scope (`r`, `b`, `cfg`, `ctx`),
  descriptive for broader scope (`serverBuildDir`, `engineVersion`).

### Constructors and Methods

Constructors use `New*` and return a pointer:
```go
func NewBuilder(opts BuildOptions, r *runner.Runner) *Builder
```

Method receivers are single-letter pointer receivers matching the type initial:
`b` for `*Builder`, `d` for `*Deployer`, `r` for `*Runner`.

`context.Context` is the first parameter for all I/O or long-running methods.

### Error Handling

- Wrap with `fmt.Errorf("brief context: %w", err)`. Lowercase, no trailing punctuation.
- No sentinel errors (`var Err*`) and no custom error types.
- Non-fatal issues: `fmt.Printf("Warning: ...")`.
- AWS errors: use `smithy.APIError` via `errors.As()` for structured error matching.

### Output

No logging library. All output via `fmt`:
- `fmt.Println` / `fmt.Printf` for status; `fmt.Fprintln(os.Stderr, ...)` for warnings.
- Stage messages indented 2 spaces. Verbose echoing via `runner.Runner` (`+ command`).
- JSON output conditional on `globals.JSONOutput`.

## Test Conventions

- **stdlib only** — no testify or assertion libraries.
- **Same-package tests** (access to unexported symbols).
- **Table-driven tests** using anonymous struct slices. Loop variable is `tt`:
  ```go
  tests := []struct{ name string; input string; want int }{ ... }
  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) { ... })
  }
  ```
- **Assertions** via `if got != want` with `t.Errorf` / `t.Fatalf`.
- **Temp dirs** via `t.TempDir()`, **env overrides** via `t.Setenv()`.
- Use `t.Chdir()` for tests that depend on the working directory.
- Test files co-located: `builder_test.go` alongside `builder.go`.

## Lint Configuration

`.golangci.yml` (v2 format). Enabled: errcheck, govet, ineffassign, staticcheck,
unused, gocritic, misspell, unconvert, gosec, dupl.

Key gosec exclusions: G104 (unhandled errors in cleanup), G204/G702 (subprocess
with variable), G304/G703 (file inclusion via variable), G115 (integer overflow),
G301/G306 (dir/file perms). ST1005 suppressed — error messages may start with
proper nouns like `Setup.sh`.

## UE Source Patches

Ludus patches UE source files at init/build time. See [UE_SOURCE_PATCHES.md](UE_SOURCE_PATCHES.md)
for full details on each patch and testing procedures.

## Feature Work

Approved feature designs and implementation plans are kept locally (not in the
public repo). Check local copies before implementing non-trivial features.
