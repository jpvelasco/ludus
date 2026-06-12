# Ludus DDC (Derived Data Cache) Management — Design Spec

**Version:** 1.1  
**Date:** 2026-04-02  
**Owner:** JP Velasco  
**Status:** Approved for Implementation

## Problem

Unreal Engine’s Derived Data Cache (DDC) is one of the biggest hidden time sinks in UE5 development, especially for dedicated server pipelines.

- Cold DDC cooks routinely take 2–4+ hours due to shader compilation, texture processing, and asset derivation.
- Every Docker-based cook in Ludus currently starts with a cold cache because containers are ephemeral.
- DDC sizes commonly reach 100–200+ GB on active projects.
- Re-derivation happens on every run, even when only small changes are made.
- DDC corruption or version mismatches after engine upgrades are common.

No existing CLI tool adequately solves DDC persistence in automated GameLift/Docker workflows.

## Goals

1. Make subsequent cooks **significantly faster** by default with zero configuration.
2. Provide a persistent DDC that survives Docker container lifecycles.
3. Support all major platforms (Linux, macOS, Windows) out of the box.
4. Keep the experience simple for solo devs while allowing advanced options for teams/CI.
5. Expose full functionality via MCP tools for AI agent orchestration.

## Non-Goals (Phase 1)

- Runtime DDC for deployed game servers (headless servers have minimal DDC usage)
- Replacing cloud storage solutions — we only automate wiring
- Caching engine source builds (C++ compilation)

## Scope

### Phase 1 (v0.1.18) — Core Persistent Local DDC
- `--ddc local` (default) and `--ddc none`
- Persistent host volume mounted at `/ddc` inside the container
- Automatic `[DerivedDataBackendGraph]` patching in `DefaultEngine.ini`
- `ludus ddc` subcommands: `status`, `clean`, `prune`, `warmup`
- MCP tools: `ludus_ddc_status`, `ludus_ddc_clean`, `ludus_ddc_configure`, `ludus_ddc_warm`
- Integration into `ludus run` and `ludus game build`

### Phase 2 (v0.1.19)
- `--ddc zen` (auto-provision Zen Storage Server)
- `--ddc s3` (bucket + IAM policy + config injection)
- Cache hit-rate reporting

## Architecture & Flow

```
ludus run / ludus game build
        ↓
  Resolve DDC config + mode
        ↓
  Create host directory (~/.ludus/ddc)
        ↓
  Add Docker volume: -v <host-path>:/ddc
        ↓
  Patch DefaultEngine.ini with [DerivedDataBackendGraph]
        ↓
  RunUAT BuildCookRun → UE reads/writes to /ddc
        ↓
  Container exits → DDC data persists on host
```

## Cross-Platform Path Handling

```go
// defaultDDCPath returns the host-side path for the local DDC
func defaultDDCPath() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("failed to resolve home directory: %w", err)
    }
    return filepath.Join(home, ".ludus", "ddc"), nil
}
```

- Linux/macOS: `~/.ludus/ddc`
- Windows: `C:\Users\Username\.ludus\ddc`

## Config & Flags

**In `ludus.yaml`** (optional):
```yaml
ddc:
  mode: local          # local | none
  local_path: ""       # override default
```

**CLI Flag** (persistent on root command):
```bash
--ddc string   # local (default), none
```

CLI flag overrides config file, which overrides default.

## Implementation Details

### 1. Volume Mount (in `internal/dockerbuild/game.go`)

When `ddc.Mode == "local"`:
- Call `defaultDDCPath()` (or use configured path)
- `os.MkdirAll(ddcHostPath, 0755)`
- Append to Docker args: `-v <hostPath>:/ddc`

### 2. Ini Patching

Create a dedicated function:

```go
// patchDerivedDataCacheConfig injects the DDC configuration into the build script
func patchDerivedDataCacheConfig(buildScript string) string
```

It should safely add the following section to `DefaultEngine.ini`:

```ini
[DerivedDataBackendGraph]
Default=Async
Async=(Type=FileSystem, Root=/ddc, ReadOnly=false)
```

Use a clean Go-based patching approach (similar to `ensureDefaultServerTarget`) rather than raw shell `sed`/`printf`. This is more reliable, testable, and easier to extend later for Zen/S3.

Hook this into both `serverBuildScript()` and `clientBuildScript()` when DDC mode is `local`.

### 3. Logging

- `DDC: local (persistent at ~/.ludus/ddc)`
- `DDC: disabled`

### 4. `ludus ddc warmup`

**Purpose**: Pre-populate **engine-level** derived data so the first real project cook is faster.

**What it should populate**:
- Core engine shaders and material templates
- Base pass / post-process shaders
- Default Lumen/global illumination data
- Standard texture compression formats

**Do NOT** cook full project content.

**Recommended RunUAT approach**:
- Target Linux server platform only
- Use a minimal/empty map
- Flags: `-NoP4 -NoCompileEditor -NoCompile`

### 5. `ludus ddc` Subcommands

- `status` → Show mode, path, size on disk
- `clean` → Delete all DDC content
- `prune` → Remove old entries (e.g. >30 days)
- `warmup` → Trigger the minimal engine cook

### 6. MCP Tools

Add 4 new tools:
- `ludus_ddc_status`
- `ludus_ddc_clean`
- `ludus_ddc_configure`
- `ludus_ddc_warm`

## Behavioral Contract

- `--ddc local` (default): mount + patch + log
- `--ddc none`: skip mount and patching
- `--dry-run`: prints the `-v` mount flag but does not execute Docker
- `--skip-game`: DDC logic is skipped

## Testing Strategy

- Unit tests for `defaultDDCPath()` on all platforms
- Unit tests for ini patch generation
- Config parsing tests (flag > yaml > default)
- Dry-run integration test
- Subcommand tests using `t.TempDir()`

## Implementation Order (Recommended)

**Foundation (do first)**
1. `DDCConfig` struct + `defaultDDCPath()`
2. `--ddc` flag on root command
3. Volume mount logic in `runBuildContainer()`
4. `patchDerivedDataCacheConfig()` + hook into build scripts
5. Logging

**Subcommands**
6. `cmd/ddc/` package with status, clean, prune, warmup

**MCP**
7. MCP tools + registration
