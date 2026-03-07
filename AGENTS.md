# AGENTS.md

Guidelines for AI coding agents working in this repository.

## Build / Lint / Test Commands

```bash
# Build
go build -o ludus -v              # Linux/macOS
go build -o ludus.exe -v .        # Windows
GOOS=windows go build -o /dev/null .  # Cross-compile check from Linux

# Lint (golangci-lint v2 required — v1 does not support Go 1.24)
golangci-lint run ./...

# Test
go test ./...                          # All tests
go test -v ./internal/toolchain        # Single package
go test -v -run TestParseBuildVersion ./internal/toolchain  # Single test
go test -v -run TestParseBuildVersion/valid_JSON ./internal/toolchain  # Single subtest

# Other
go vet ./...                      # Static analysis
go mod tidy                       # Clean up module dependencies
```

Pre-commit hooks (`.hooks/pre-commit`) run `go build ./...`, lint, and `go test ./...`.
Activate with `git config core.hooksPath .hooks`.

## Project Structure

- `main.go` — Entry point; calls `root.Execute()`.
- `cmd/` — Cobra command packages. Each command package exports `var Cmd = &cobra.Command{...}`,
  registered in `cmd/root/root.go` via `rootCmd.AddCommand(...)` in `init()`.
  Handler functions are named `run<Command>`.
  - **Exceptions**: `cmd/globals/` exports mutable global state (`Cfg`, `Verbose`,
    `JSONOutput`, `DryRun`), not a `Cmd`. The `init` command is defined as an
    unexported `initCmd` in `cmd/root/init.go`.
  - **Subcommand groups**: Some commands are groups with subcommands (e.g., `deploy`
    has `fleet`, `stack`, `session`, `anywhere`, `destroy`; `engine` has `build`,
    `setup`, `push`). These `Cmd` vars have no `RunE` of their own.
  - `cmd/mcp/` — MCP (Model Context Protocol) server for AI agent orchestration
    via stdio JSON-RPC.
- `internal/` — All business logic (unexported). One primary type per file.
  Key packages: `config`, `runner`, `engine`, `game`, `container`, `deploy`,
  `gamelift`, `stack`, `ec2fleet`, `binary`, `anywhere`, `tags`, `state`, `cache`,
  `status`, `prereq`, `toolchain`, `wrapper`, `ci`, `dockerbuild`, `buildgraph`,
  `pricing`, `diagnose`, `dflint`, `progress`.
- Config loaded via Viper from `ludus.yaml`.
- Platform-specific files use `_windows.go` / `_unix.go` suffixes with `//go:build` tags.

## Code Style

### Formatting & Imports

Enforced by `gofmt`. Two import groups separated by a blank line: (1) stdlib,
(2) everything else (third-party and project imports together, sorted alphabetically).

```go
import (
    "context"
    "fmt"

    "github.com/devrecon/ludus/internal/runner"
    "github.com/spf13/cobra"
)
```

Aliases only to resolve naming conflicts, using concise names. Common patterns
include AWS type packages (`gltypes`, `cftypes`, `iamtypes`) and cmd-vs-internal
disambiguation (`engBuilder`, `gameBuilder`, `ctrBuilder`, `internalstatus`).

### Naming

- **Packages**: lowercase, single word. Multi-word concatenated (`dockerbuild`).
- **Files**: `snake_case.go`. Build-tagged: `checker_unix.go`, `process_windows.go`.
- **Acronyms**: Fully uppercase: `ID`, `URI`, `ARN`, `ECR`, `IAM`, `AWS`, `SDK`, `IP`.
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
`b` for `*Builder`, `d` for `*Deployer`, `r` for `*Runner`, `c` for `*Checker`.

`context.Context` is the first parameter for all I/O or long-running methods.
Pure computation functions omit it.

### Error Handling

- Wrap with `fmt.Errorf("brief context: %w", err)`. Context is lowercase, no
  trailing punctuation: `"reading config: %w"`, `"creating location: %w"`.
- Terminal errors (no underlying cause) use `fmt.Errorf` without `%w`.
- No sentinel errors (`var Err*`) and no custom error types — all errors are
  `fmt.Errorf` or from library calls.
- Non-fatal issues: `fmt.Printf("Warning: failed to write state: %v\n", err)`.
- AWS not-found checks use `smithy.APIError` type assertion (`errors.As()`) for structured error matching.

### Comments & Output

Doc comments on all exported identifiers, starting with the identifier name.
Struct fields get inline `//` comments in config types. Inline comments explain "why".

No logging library. All output via `fmt`:
- `fmt.Println` / `fmt.Printf` for status; `fmt.Fprintln(os.Stderr, ...)` for warnings.
- Stage messages indented 2 spaces. Verbose echoing via `runner.Runner` (`+ command`).
- JSON output conditional on `globals.JSONOutput`.

## Test Conventions

- **stdlib only** — no testify or assertion libraries.
- **Same-package tests** (access to unexported symbols): `package toolchain`, not
  `package toolchain_test`.
- **Table-driven tests** using anonymous struct slices. Loop variable is `tt`:
  ```go
  tests := []struct{ name string; input string; want int }{ ... }
  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) { ... })
  }
  ```
- **Assertions** via `if got != want` with `t.Errorf` / `t.Fatalf`.
- **Temp dirs** via `t.TempDir()`, **env overrides** via `t.Setenv()` (both auto-cleaned).
- **Skip** unavailable tests with `t.Skipf(...)`.
- Test files are co-located: `builder_test.go` alongside `builder.go`.

## Lint Configuration

Enabled linters (`.golangci.yml`, v2 format): errcheck, govet, ineffassign,
staticcheck, unused, gocritic, misspell, unconvert, gosec.

Key gosec exclusions: G104 (unhandled errors in cleanup), G204/G702 (subprocess
with variable), G304/G703 (file inclusion via variable), G115 (integer overflow),
G301/G306 (dir/file perms). ST1005 suppressed — error messages may start with
proper nouns like `Setup.sh`.

## Key Patterns

- **Builder pattern**: `New*(opts)` constructor, operation methods, structured results.
- **Runner abstraction**: Never call `exec.Command` directly — use `runner.Runner`
  which handles verbose/dry-run uniformly.
- **Dual build backends**: Engine and game builds support both `native` and `docker`
  backends. See `internal/dockerbuild/`.
- **Pluggable targets**: `deploy.Target` interface with `gamelift`, `stack`, `ec2`,
  `binary`, `anywhere` implementations. Factory in `cmd/globals/resolve.go`.
- **State persistence**: `.ludus/state.json` for fleet/session/client info.
- **Build caching**: `.ludus/cache.json` with input hashing per stage.
- **Platform dispatch**: `//go:build windows` / `//go:build !windows` pairs with
  matching function signatures. Minor differences use `runtime.GOOS` inline.

## UE Source Patches

Ludus patches UE source files at init/build time. See `UE_SOURCE_PATCHES.md` for
full details on each patch and testing procedures. Use `scripts/validate_ue_versions.sh`
for multi-version structural validation against UE source tarballs.
