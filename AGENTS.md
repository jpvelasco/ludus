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

Pre-commit hooks (`.hooks/pre-commit`) run build, lint, and test. Activate with
`git config core.hooksPath .hooks`.

## Project Structure

- `main.go` — Entry point; calls `root.Execute()`.
- `cmd/` — Cobra command packages. Each exports `var Cmd = &cobra.Command{...}`,
  registered in `cmd/root/root.go`. Handler functions are named `run<Command>`.
- `cmd/globals/` — Mutable global state: `Cfg`, `Verbose`, `JSONOutput`, `DryRun`.
- `internal/` — All business logic (unexported). One primary type per file.
  Key packages: `config`, `runner`, `engine`, `game`, `container`, `deploy`,
  `gamelift`, `stack`, `binary`, `anywhere`, `tags`, `state`, `cache`, `status`,
  `prereq`, `toolchain`, `wrapper`, `ci`, `dockerbuild`.
- Platform-specific files use `_windows.go` / `_unix.go` suffixes with `//go:build` tags.

## Code Style

### Formatting

Enforced by `gofmt` (configured in `.golangci.yml`). No manual formatting exceptions.

### Imports

Two groups separated by a blank line: (1) stdlib, (2) everything else (third-party
and project imports together). Sorted alphabetically within each group.

```go
import (
    "context"
    "fmt"

    "github.com/devrecon/ludus/internal/runner"
    "github.com/spf13/cobra"
)
```

Aliases only to resolve naming conflicts, using concise names:

```go
awsconfig "github.com/aws/aws-sdk-go-v2/config"
gltypes   "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
```

### Naming

- **Packages**: lowercase, single word (`config`, `deploy`, `gamelift`). Multi-word
  concatenated (`dockerbuild`), never underscored.
- **Files**: `snake_case.go`. Build-tagged files: `checker_unix.go`, `process_windows.go`.
- **Acronyms**: Fully uppercase: `ID`, `URI`, `ARN`, `ECR`, `IAM`, `AWS`, `SDK`, `IP`.
- **Structs**: PascalCase nouns — `Builder`, `Deployer`, `Exporter`, `TargetAdapter`.
- **Options/Results**: `BuildOptions`, `BuildResult`, `DeployOptions`, `FleetStatus`.
- **Unexported constants**: camelCase (`iamRoleName`, `pollInterval`).
- **Exported constants**: PascalCase (`WrapperRepo`, `StageEngine`).
- **Variables**: camelCase. Short names for narrow scope (`r`, `b`, `cfg`, `ctx`).
  Descriptive names for broader scope (`serverBuildDir`, `engineVersion`).

### Constructors and Methods

Constructors use `New*` and return a pointer:

```go
func NewBuilder(opts BuildOptions, r *runner.Runner) *Builder
func NewDeployer(opts DeployOptions, awsCfg aws.Config) *Deployer
```

Method receivers are pointer receivers with a single-letter name matching the type:
`b` for `*Builder`, `d` for `*Deployer`, `r` for `*Runner`, `c` for `*Checker`.

### Context

`context.Context` is the first parameter for all I/O or long-running methods.
Pure computation functions omit it.

```go
func (b *Builder) Build(ctx context.Context) (*BuildResult, error)
func (d *Deployer) CreateFleet(ctx context.Context, ...) (...)
```

### Error Handling

- Wrap with `fmt.Errorf("brief context: %w", err)`. Context is lowercase, no trailing
  punctuation: `"reading config: %w"`, `"creating location: %w"`.
- Terminal errors (no underlying cause) use `fmt.Errorf` without `%w`:
  `fmt.Errorf("Setup.sh not found at %s", path)`.
- No sentinel errors (`var Err* = errors.New(...)`) — the codebase does not use them.
- No custom error types — all errors are `fmt.Errorf` or from library calls.
- Non-fatal issues print a warning and continue:
  `fmt.Printf("Warning: failed to write state: %v\n", err)`.
- AWS not-found checks use string matching helpers (`isNotFound()`), not `errors.Is()`.

### Comments

Doc comments on all exported identifiers, starting with the identifier name:

```go
// Builder compiles UE5 from source.
// NewBuilder creates a new engine builder.
// BuildOptions configures the engine build.
```

Struct fields get inline `//` comments in config types. Inline code comments explain
"why", not "what".

### Output / Logging

No logging library. All output via `fmt` functions:
- `fmt.Println` / `fmt.Printf` for status messages.
- `fmt.Fprintln(os.Stderr, ...)` for warnings to stderr.
- Status messages within stages are indented 2 spaces.
- Verbose command echoing is handled by the `runner.Runner` (prints `+ command`).
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
- **Temp dirs** via `t.TempDir()` (auto-cleaned).
- **Skip** unavailable tests with `t.Skipf(...)`.
- Test files are co-located: `builder_test.go` alongside `builder.go`.

## Lint Configuration

Enabled linters (`.golangci.yml`, v2 format): errcheck, govet, ineffassign,
staticcheck, unused, gocritic, misspell, unconvert, gosec.

Key gosec exclusions: G104 (unhandled errors in best-effort cleanup), G204/G702
(subprocess with variable — intentional), G304/G703 (file inclusion via variable —
intentional), G115 (integer overflow — bounded values), G301 (dir perms 0755),
G306 (WriteFile 0644).

ST1005 (uppercase error strings) is suppressed — error messages may start with proper
nouns like `Setup.sh` or `Lyra.uproject`.

## Key Patterns

- **Builder pattern**: `New*(opts)` constructor, operation methods, structured results.
- **Runner abstraction**: Never call `exec.Command` directly — use `runner.Runner`
  which handles verbose/dry-run uniformly.
- **Pluggable targets**: `deploy.Target` interface with `gamelift`, `stack`, `binary`,
  `anywhere` implementations. Factory in `cmd/globals/resolve.go`.
- **State persistence**: `.ludus/state.json` for fleet/session/client info.
- **Build caching**: `.ludus/cache.json` with input hashing per stage.
- **Platform dispatch**: `//go:build windows` / `//go:build !windows` pairs providing
  matching function signatures. Minor differences use `runtime.GOOS` inline.

## Testing UE Source Patches

Ludus applies patches to UE source files at init/build time. To validate these
against real UE source, download releases via GitHub CLI (requires Epic Games
account linked to GitHub):

```bash
# Download a specific UE release
gh release download 5.6.1-release --repo EpicGames/UnrealEngine --archive tar.gz -O ue-5.6.1.tar.gz
```

### INITGUID auto-fix (Windows + UE 5.6 only)

The auto-fix in `internal/prereq/checker_windows.go` patches
`NNERuntimeORT.Build.cs` to add `INITGUID` after `ORT_USE_NEW_DXCORE_FEATURES`.
It only triggers on Windows SDK >= 26100 with engine version 5.6 (skipped on
5.4, 5.5, 5.7). To test:

1. Extract `NNERuntimeORT.Build.cs` from a UE 5.6 tarball
2. Point `engine.sourcePath` at the extracted tree
3. Run `ludus init --fix --verbose`
4. Verify the patched file has `PublicDefinitions.Add("INITGUID");` on the line
   after `PublicDefinitions.Add("ORT_USE_NEW_DXCORE_FEATURES");`

### Full multi-version structural validation (Linux)

`scripts/validate_ue_versions.sh` checks Ludus's assumptions (file paths,
markers, plugin structure) against multiple UE source tarballs without building.
Place tarballs named `UnrealEngine-X.Y.Z-release.tar.gz` in `~/Downloads/` and
run:

```bash
bash scripts/validate_ue_versions.sh ~/Downloads
```

See `UE_SOURCE_PATCHES.md` for full details on each patch and testing procedures.
