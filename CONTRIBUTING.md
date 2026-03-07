# Contributing to Ludus

Thanks for your interest in contributing! Ludus is open source under the MIT license and we welcome contributions of all kinds.

## Getting Started

1. Fork the repository
2. Clone your fork and create a branch:
   ```bash
   git clone git@github.com:<your-username>/ludus.git
   cd ludus
   git checkout -b feat/your-feature
   ```
3. Install dependencies:
   - Go 1.24+
   - golangci-lint v2 (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`)
4. Activate pre-commit hooks:
   ```bash
   git config core.hooksPath .hooks
   ```

## Development Workflow

```bash
go build -o ludus.exe -v .        # Build (Windows)
go build -o ludus -v .            # Build (Linux/macOS)
go vet ./...                      # Static analysis
golangci-lint run ./...           # Lint (must pass)
go test ./...                     # Run all tests
go test -v ./internal/toolchain   # Run a single package
```

The pre-commit hook runs build, lint, and tests automatically. All three must pass before committing.

## Pull Request Process

1. Create a branch from `main` with a descriptive name (`feat/`, `fix/`, `docs/`)
2. Make your changes — keep PRs focused on a single concern
3. Ensure `golangci-lint run ./...` and `go test ./...` pass
4. Write a clear PR description explaining **what** changed and **why**
5. CI runs lint, build, and test on both Ubuntu and Windows — all must pass

## Code Style

- Follow existing patterns — see [AGENTS.md](AGENTS.md) for detailed conventions
- Use `runner.Runner` for shell execution (never raw `exec.Command`)
- Two import groups: stdlib, then everything else
- Table-driven tests with stdlib only (no testify)
- Error wrapping: `fmt.Errorf("brief context: %w", err)`

## What to Contribute

- Bug fixes with reproduction steps
- Documentation improvements
- Test coverage for uncovered packages
- New deployment target implementations (via `deploy.Target` interface)
- Platform support improvements (WSL2, macOS)

## Reporting Issues

Use [GitHub Issues](https://github.com/jpvelasco/ludus/issues). For bugs, include:
- Ludus version (`ludus --version` or `git rev-parse --short HEAD`)
- OS and UE version
- Steps to reproduce
- Full error output (with `--verbose`)

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you agree to uphold it.
