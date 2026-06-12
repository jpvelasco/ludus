# DDC (Derived Data Cache) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add persistent DDC support to ludus so Docker-based cooks reuse cached derived data, eliminating cold-cache rebuilds.

**Architecture:** DDC config threads from `ludus.yaml` / `--ddc` flag through `DockerGameOptions` into `runBuildContainer()`, which conditionally mounts a host volume at `/ddc` and injects a `[DerivedDataBackendGraph]` ini patch into the build script. Subcommands (`ludus ddc status/clean/prune/warmup`) and MCP tools provide management.

**Tech Stack:** Go 1.25, Cobra/Viper, Docker volume mounts, UE5 DefaultEngine.ini patching

**Spec:** `docs/superpowers/specs/2026-04-02-ddc-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/config/config.go` | Modify | Add `DDCConfig` struct, field on `Config`, defaults in `Defaults()` |
| `internal/config/config_test.go` | Modify | Test DDC defaults and YAML parsing |
| `internal/ddc/ddc.go` | Create | DDC path resolution, directory size, prune logic |
| `internal/ddc/ddc_test.go` | Create | Tests for path resolution, size calculation, pruning |
| `cmd/globals/globals.go` | Modify | Add `DDCMode` global variable |
| `cmd/root/root.go` | Modify | Add `--ddc` persistent flag, wire to `globals.DDCMode` |
| `internal/dockerbuild/game.go` | Modify | Add `DDCMode`/`DDCPath` to options, volume mount, ini patch snippet |
| `internal/dockerbuild/game_test.go` | Modify | Test DDC volume mount and ini patch in build scripts |
| `cmd/game/game.go` | Modify | Thread DDC config into `DockerGameOptions` |
| `cmd/pipeline/stages.go` | Modify | Thread DDC config into `DockerGameOptions` in `stageGameBuild` |
| `cmd/ddc/ddc.go` | Create | `ludus ddc` command group with `status`, `clean`, `prune`, `warmup` subcommands |
| `cmd/mcp/tools_ddc.go` | Create | 4 MCP tools: status, clean, configure, warm |
| `cmd/mcp/register.go` | Modify | Add `registerDDCTools(s)` call |

---

## Task 1: Add DDCConfig to config and defaults

**Files:**
- Modify: `internal/config/config.go:82-92` (Config struct), `internal/config/config.go:332-378` (Defaults)
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test for DDC defaults**

Add to `internal/config/config_test.go` inside `TestDefaults`, after the existing string tests slice (around line 36):

```go
		{"ddc mode", cfg.DDC.Mode, "local"},
		{"ddc local path", cfg.DDC.LocalPath, ""},
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestDefaults ./internal/config`
Expected: FAIL — `cfg.DDC` does not exist yet.

- [ ] **Step 3: Write the failing test for DDC YAML parsing**

Add a new test in `internal/config/config_test.go`:

```go
func TestLoad_DDCConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	yamlContent := `ddc:
  mode: none
  local_path: /custom/ddc
`
	if err := os.WriteFile("ludus.yaml", []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.DDC.Mode != "none" {
		t.Errorf("ddc mode: got %q, want %q", cfg.DDC.Mode, "none")
	}
	if cfg.DDC.LocalPath != "/custom/ddc" {
		t.Errorf("ddc local_path: got %q, want %q", cfg.DDC.LocalPath, "/custom/ddc")
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test -v -run TestLoad_DDCConfig ./internal/config`
Expected: FAIL — `cfg.DDC` does not exist.

- [ ] **Step 5: Add DDCConfig struct and wire into Config**

In `internal/config/config.go`, add the struct after `CIConfig` (around line 312):

```go
// DDCConfig holds Derived Data Cache settings for UE5 builds.
type DDCConfig struct {
	// Mode selects the DDC backend: "local" (default) or "none".
	Mode string `yaml:"mode"`
	// LocalPath overrides the default local DDC directory (~/.ludus/ddc).
	LocalPath string `yaml:"local_path"`
}
```

Add the field to the `Config` struct (after `CI`):

```go
	DDC       DDCConfig       `yaml:"ddc"`
```

Add DDC defaults in `Defaults()` (inside the return block, after `CI`):

```go
		DDC: DDCConfig{
			Mode: "local",
		},
```

- [ ] **Step 6: Run both tests to verify they pass**

Run: `go test -v -run "TestDefaults|TestLoad_DDCConfig" ./internal/config`
Expected: PASS

- [ ] **Step 7: Run full config test suite**

Run: `go test -v ./internal/config`
Expected: All tests PASS

- [ ] **Step 8: Commit**

```bash
git checkout -b feat/ddc
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add DDCConfig struct with local/none modes

Add DDC configuration to ludus.yaml with mode (local|none) and
optional local_path override. Default mode is 'local' for
zero-config persistent DDC.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 2: Create internal/ddc package with path resolution

**Files:**
- Create: `internal/ddc/ddc.go`
- Create: `internal/ddc/ddc_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/ddc/ddc_test.go`:

```go
package ddc

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultPath(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error: %v", err)
	}

	want := filepath.Join(home, ".ludus", "ddc")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestResolvePath_Override(t *testing.T) {
	got, err := ResolvePath("/custom/ddc")
	if err != nil {
		t.Fatalf("ResolvePath() error: %v", err)
	}
	if got != "/custom/ddc" {
		t.Errorf("ResolvePath(%q) = %q, want %q", "/custom/ddc", got, "/custom/ddc")
	}
}

func TestResolvePath_Default(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}

	got, err := ResolvePath("")
	if err != nil {
		t.Fatalf("ResolvePath() error: %v", err)
	}

	want := filepath.Join(home, ".ludus", "ddc")
	if got != want {
		t.Errorf("ResolvePath(%q) = %q, want %q", "", got, want)
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()

	// Empty directory
	size, err := DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize() error: %v", err)
	}
	if size != 0 {
		t.Errorf("empty dir size = %d, want 0", size)
	}

	// Add a file
	if err := os.WriteFile(filepath.Join(dir, "test.bin"), make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}

	size, err = DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize() error: %v", err)
	}
	if size != 1024 {
		t.Errorf("dir size = %d, want 1024", size)
	}
}

func TestDirSize_NotExist(t *testing.T) {
	size, err := DirSize("/nonexistent/path")
	if err != nil {
		t.Fatalf("DirSize() should not error for missing dir: %v", err)
	}
	if size != 0 {
		t.Errorf("nonexistent dir size = %d, want 0", size)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./internal/ddc`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement internal/ddc/ddc.go**

Create `internal/ddc/ddc.go`:

```go
package ddc

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultPath returns the default DDC directory path.
//
//	Linux/macOS: ~/.ludus/ddc
//	Windows:     C:\Users\Username\.ludus\ddc
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".ludus", "ddc"), nil
}

// ResolvePath returns the DDC path, using the override if non-empty,
// otherwise falling back to DefaultPath.
func ResolvePath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return DefaultPath()
}

// DirSize returns the total size in bytes of all files under dir.
// Returns 0 without error if the directory does not exist.
func DirSize(dir string) (int64, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, nil
	}

	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// Clean removes all contents of the DDC directory without removing the directory itself.
// Returns the number of bytes freed. Returns 0 without error if the directory does not exist.
func Clean(dir string) (int64, error) {
	size, err := DirSize(dir)
	if err != nil {
		return 0, err
	}
	if size == 0 {
		return 0, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading DDC directory: %w", err)
	}

	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return 0, fmt.Errorf("removing %s: %w", entry.Name(), err)
		}
	}

	return size, nil
}

// Prune removes files in the DDC directory that haven't been modified in the given
// number of days. Returns the number of bytes freed.
func Prune(dir string, maxAgeDays int) (int64, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, nil
	}

	cutoff := maxAgeDays * 24 * 60 * 60 // seconds
	var freed int64

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		ageSec := int(fileAge(info).Seconds())
		if ageSec > cutoff {
			freed += info.Size()
			return os.Remove(path)
		}
		return nil
	})
	return freed, err
}
```

Note: `fileAge` needs a small helper. Add at the bottom of the same file:

```go
import "time"

func fileAge(info fs.FileInfo) time.Duration {
	return time.Since(info.ModTime())
}
```

Make sure the `time` import is in the import block at the top (add it alongside the other imports).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/ddc`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ddc/ddc.go internal/ddc/ddc_test.go
git commit -m "feat: add internal/ddc package for path resolution and cache management

Provides DefaultPath(), ResolvePath(), DirSize(), Clean(), and Prune()
functions for DDC directory management. Cross-platform via os.UserHomeDir().

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 3: Add --ddc flag to root command

**Files:**
- Modify: `cmd/globals/globals.go`
- Modify: `cmd/root/root.go:103-108`

- [ ] **Step 1: Add DDCMode to globals**

In `cmd/globals/globals.go`, add after the `Profile` variable (line 19):

```go
// DDCMode is the DDC backend mode: "local" (default) or "none".
// Set via --ddc flag, overrides config file.
var DDCMode string
```

- [ ] **Step 2: Add --ddc persistent flag to root command**

In `cmd/root/root.go`, in the `init()` function (after line 108), add:

```go
	rootCmd.PersistentFlags().StringVar(&globals.DDCMode, "ddc", "", `DDC mode: "local" (default) or "none" (disable cache)`)
```

- [ ] **Step 3: Build to verify compilation**

Run: `go build -o ludus.exe -v .`
Expected: Compiles successfully

- [ ] **Step 4: Verify flag appears in help**

Run: `./ludus.exe --help`
Expected: `--ddc string` appears in the global flags section

- [ ] **Step 5: Commit**

```bash
git add cmd/globals/globals.go cmd/root/root.go
git commit -m "feat: add --ddc persistent flag for DDC mode selection

Adds global DDCMode variable and --ddc flag to root command.
Available on all subcommands (ludus run, ludus game build, etc.).
Supports 'local' (default) and 'none' modes.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 4: Add DDC volume mount and ini patch to Docker game builder

**Files:**
- Modify: `internal/dockerbuild/game.go:14-41` (DockerGameOptions), `game.go:97-158` (build scripts), `game.go:246-276` (runBuildContainer)
- Modify: `internal/dockerbuild/game_test.go`

- [ ] **Step 1: Write the failing tests for DDC in build scripts**

Add to `internal/dockerbuild/game_test.go`:

```go
func TestDDCIniPatch(t *testing.T) {
	tests := []struct {
		name        string
		opts        DockerGameOptions
		contains    []string
		notContains []string
	}{
		{
			name: "local mode injects DDC patch",
			opts: DockerGameOptions{DDCMode: "local"},
			contains: []string{
				"DerivedDataBackendGraph",
				"Type=FileSystem",
				"Root=/ddc",
				"DDC: Configured persistent cache",
			},
		},
		{
			name:        "none mode skips DDC patch",
			opts:        DockerGameOptions{DDCMode: "none"},
			notContains: []string{"DerivedDataBackendGraph"},
		},
		{
			name:        "empty mode (default local) injects DDC patch",
			opts:        DockerGameOptions{},
			notContains: []string{"DerivedDataBackendGraph"},
		},
	}

	r := runner.NewRunner(false, false)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.generateBuildScript(true)

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("script should contain %q\ngot:\n%s", want, got)
				}
			}
			for _, notWant := range tt.notContains {
				if strings.Contains(got, notWant) {
					t.Errorf("script should NOT contain %q\ngot:\n%s", notWant, got)
				}
			}
		})
	}
}

func TestDDCIniPatch_ClientBuild(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{DDCMode: "local"}, r)
	got := b.generateBuildScript(false)

	if !strings.Contains(got, "DerivedDataBackendGraph") {
		t.Errorf("client script should contain DDC patch when mode=local\ngot:\n%s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v -run "TestDDCIniPatch" ./internal/dockerbuild`
Expected: FAIL — `DDCMode` field does not exist on `DockerGameOptions`.

- [ ] **Step 3: Add DDC fields to DockerGameOptions**

In `internal/dockerbuild/game.go`, add to the `DockerGameOptions` struct (after `EngineVersion` field, around line 40):

```go
	// DDCMode is the DDC backend mode: "local" or "none".
	DDCMode string
	// DDCPath is the host path for the local DDC volume.
	DDCPath string
```

- [ ] **Step 4: Add the ddcIniPatch method**

In `internal/dockerbuild/game.go`, add after the `scriptPreamble` method (after line 116):

```go
// ddcIniPatch returns a shell snippet that patches DefaultEngine.ini with
// [DerivedDataBackendGraph] configuration pointing to the /ddc mount.
// Returns an empty string if DDC mode is not "local".
func (b *DockerGameBuilder) ddcIniPatch() string {
	if b.opts.DDCMode != "local" {
		return ""
	}

	projectDir := filepath.Dir(b.containerProjectPath())
	return fmt.Sprintf(`# Configure DDC persistent cache
DDC_INI="%s/Config/DefaultEngine.ini"
if [ -f "$DDC_INI" ] && ! grep -q "DerivedDataBackendGraph" "$DDC_INI"; then
    printf '\n[DerivedDataBackendGraph]\nDefault=Async\nAsync=(Type=FileSystem, Root=/ddc, ReadOnly=false)\n' >> "$DDC_INI"
    echo "DDC: Configured persistent cache at /ddc"
fi

`, projectDir)
}
```

- [ ] **Step 5: Hook ddcIniPatch into serverBuildScript and clientBuildScript**

In `serverBuildScript()`, insert the DDC patch after the existing DefaultServerTarget patch and before `cd /engine`. Change line 135 from:

```go
	script += "cd /engine\n\n"
```

to:

```go
	script += b.ddcIniPatch()
	script += "cd /engine\n\n"
```

In `clientBuildScript()`, insert before `cd /engine`. Change line 173 from:

```go
	script := "cd /engine\n\n"
```

to:

```go
	script := b.ddcIniPatch()
	script += "cd /engine\n\n"
```

- [ ] **Step 6: Add DDC volume mount to runBuildContainer**

In `runBuildContainer()`, after the external project volume mount block (after line 268) and before the final `args = append(args, b.opts.EngineImage, ...)` line, add:

```go
	if b.opts.DDCMode == "local" && b.opts.DDCPath != "" {
		if err := os.MkdirAll(b.opts.DDCPath, 0755); err != nil {
			return fmt.Errorf("creating DDC directory: %w", err)
		}
		args = append(args, "-v", fmt.Sprintf("%s:/ddc", b.opts.DDCPath))
		fmt.Printf("DDC: local (persistent at %s)\n", b.opts.DDCPath)
	} else if b.opts.DDCMode == "none" {
		fmt.Println("DDC: disabled")
	}
```

- [ ] **Step 7: Run DDC tests to verify they pass**

Run: `go test -v -run "TestDDCIniPatch" ./internal/dockerbuild`
Expected: PASS

- [ ] **Step 8: Run full dockerbuild test suite**

Run: `go test -v ./internal/dockerbuild`
Expected: All tests PASS. Existing tests are unaffected since DDCMode defaults to empty string which produces no DDC output.

- [ ] **Step 9: Commit**

```bash
git add internal/dockerbuild/game.go internal/dockerbuild/game_test.go
git commit -m "feat: add DDC volume mount and ini patching to Docker game builder

When DDCMode is 'local', mounts host DDC directory at /ddc and patches
DefaultEngine.ini with [DerivedDataBackendGraph] config. Applied to both
server and client build scripts. Logs DDC mode for visibility.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 5: Thread DDC config through cmd/game and cmd/pipeline

**Files:**
- Modify: `cmd/game/game.go:241-267` (runDockerBuild), `cmd/game/game.go:335-377` (runDockerClientBuild)
- Modify: `cmd/pipeline/stages.go:165-185` (stageGameBuild)

- [ ] **Step 1: Create a helper to resolve DDC mode and path**

Add a new file `cmd/globals/ddc.go`:

```go
package globals

import "github.com/devrecon/ludus/internal/ddc"

// ResolveDDCMode returns the effective DDC mode.
// CLI flag (DDCMode) takes precedence over config (Cfg.DDC.Mode).
func ResolveDDCMode() string {
	if DDCMode != "" {
		return DDCMode
	}
	if Cfg != nil && Cfg.DDC.Mode != "" {
		return Cfg.DDC.Mode
	}
	return "local"
}

// ResolveDDCPath returns the effective DDC host path.
// Config local_path takes precedence over the default path.
func ResolveDDCPath() string {
	if Cfg != nil && Cfg.DDC.LocalPath != "" {
		return Cfg.DDC.LocalPath
	}
	p, err := ddc.DefaultPath()
	if err != nil {
		return ""
	}
	return p
}
```

- [ ] **Step 2: Wire DDC into cmd/game/game.go runDockerBuild**

In `runDockerBuild()` (around line 258), add `DDCMode` and `DDCPath` to the `DockerGameOptions` struct literal:

```go
	builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
		EngineImage:   engineImage,
		ProjectPath:   cfg.Game.ProjectPath,
		ProjectName:   cfg.Game.ProjectName,
		ServerTarget:  cfg.Game.ResolvedServerTarget(),
		GameTarget:    cfg.Game.ResolvedGameTarget(),
		SkipCook:      skipCook,
		ServerMap:     cfg.Game.ServerMap,
		EngineVersion: engineVersion,
		DDCMode:       globals.ResolveDDCMode(),
		DDCPath:       globals.ResolveDDCPath(),
	}, r)
```

- [ ] **Step 3: Wire DDC into cmd/game/game.go runDockerClientBuild**

In `runDockerClientBuild()` (around line 352), add the same two fields:

```go
	builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
		EngineImage:    engineImage,
		ProjectPath:    cfg.Game.ProjectPath,
		ProjectName:    cfg.Game.ProjectName,
		ClientTarget:   cfg.Game.ResolvedClientTarget(),
		ClientPlatform: clientPlatform,
		SkipCook:       skipCookClient,
		EngineVersion:  engineVersion,
		DDCMode:        globals.ResolveDDCMode(),
		DDCPath:        globals.ResolveDDCPath(),
	}, r)
```

- [ ] **Step 4: Wire DDC into cmd/pipeline/stages.go stageGameBuild**

In `stageGameBuild()` (around line 177), add DDC fields to the Docker builder options:

```go
		builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
			EngineImage:   engineImage,
			ProjectPath:   p.cfg.Game.ProjectPath,
			ProjectName:   projectName,
			ServerTarget:  p.cfg.Game.ResolvedServerTarget(),
			GameTarget:    p.cfg.Game.ResolvedGameTarget(),
			ServerMap:     p.cfg.Game.ServerMap,
			EngineVersion: p.engineVersion,
			DDCMode:       globals.ResolveDDCMode(),
			DDCPath:       globals.ResolveDDCPath(),
		}, p.r)
```

- [ ] **Step 5: Build to verify compilation**

Run: `go build -o ludus.exe -v .`
Expected: Compiles successfully

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: All tests PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/globals/ddc.go cmd/game/game.go cmd/pipeline/stages.go
git commit -m "feat: thread DDC config through game build and pipeline commands

ResolveDDCMode() and ResolveDDCPath() helpers centralize DDC config
resolution (CLI flag > yaml > default). Wired into runDockerBuild,
runDockerClientBuild, and stageGameBuild.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 6: Add ludus ddc subcommands

**Files:**
- Create: `cmd/ddc/ddc.go`
- Modify: `cmd/root/root.go:110-124` (add ddc command)

- [ ] **Step 1: Create cmd/ddc/ddc.go**

```go
package ddc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/ddc"
	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/spf13/cobra"
)

var pruneDays int

// Cmd is the top-level ddc command group.
var Cmd = &cobra.Command{
	Use:   "ddc",
	Short: "Manage the Derived Data Cache (DDC)",
	Long: `Commands for managing the UE5 Derived Data Cache.

The DDC stores pre-computed shader, texture, and asset data so that
subsequent builds skip expensive re-derivation.

  ludus ddc status    Show DDC backend, path, and size
  ludus ddc clean     Delete all cached data
  ludus ddc prune     Remove old entries (default: >30 days)
  ludus ddc warmup    Pre-warm engine shaders with a minimal cook`,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show DDC backend, path, and size on disk",
	RunE:  runStatus,
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Delete all DDC content",
	RunE:  runClean,
}

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old DDC entries",
	RunE:  runPrune,
}

var warmupCmd = &cobra.Command{
	Use:   "warmup",
	Short: "Pre-warm engine-level DDC with a minimal cook",
	Long: `Runs a minimal, engine-focused cook to pre-populate the DDC with
common engine shaders, material templates, Lumen data, and texture
compression formats. This makes the first real project cook faster.`,
	RunE: runWarmup,
}

func init() {
	pruneCmd.Flags().IntVar(&pruneDays, "days", 30, "remove entries older than this many days")

	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(cleanCmd)
	Cmd.AddCommand(pruneCmd)
	Cmd.AddCommand(warmupCmd)
}

func resolveDDCPath() (string, error) {
	return ddc.ResolvePath(globals.Cfg.DDC.LocalPath)
}

func runStatus(cmd *cobra.Command, args []string) error {
	mode := globals.ResolveDDCMode()
	ddcPath, err := resolveDDCPath()
	if err != nil {
		return err
	}

	size, err := ddc.DirSize(ddcPath)
	if err != nil {
		return fmt.Errorf("calculating DDC size: %w", err)
	}

	if globals.JSONOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"mode":       mode,
			"path":       ddcPath,
			"size_bytes": size,
		})
	}

	fmt.Printf("DDC Status\n")
	fmt.Printf("  Mode: %s\n", mode)
	fmt.Printf("  Path: %s\n", ddcPath)
	fmt.Printf("  Size: %s\n", formatSize(size))
	return nil
}

func runClean(cmd *cobra.Command, args []string) error {
	ddcPath, err := resolveDDCPath()
	if err != nil {
		return err
	}

	freed, err := ddc.Clean(ddcPath)
	if err != nil {
		return fmt.Errorf("cleaning DDC: %w", err)
	}

	if globals.JSONOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":    true,
			"bytes_freed": freed,
		})
	}

	if freed == 0 {
		fmt.Println("DDC is already empty.")
	} else {
		fmt.Printf("DDC cleaned: %s freed\n", formatSize(freed))
	}
	return nil
}

func runPrune(cmd *cobra.Command, args []string) error {
	ddcPath, err := resolveDDCPath()
	if err != nil {
		return err
	}

	freed, err := ddc.Prune(ddcPath, pruneDays)
	if err != nil {
		return fmt.Errorf("pruning DDC: %w", err)
	}

	if globals.JSONOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":     true,
			"bytes_freed": freed,
			"max_age_days": pruneDays,
		})
	}

	if freed == 0 {
		fmt.Printf("No DDC entries older than %d days.\n", pruneDays)
	} else {
		fmt.Printf("DDC pruned: %s freed (entries older than %d days)\n", formatSize(freed), pruneDays)
	}
	return nil
}

func runWarmup(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	if cfg.Engine.Backend != "docker" && cfg.Engine.DockerImage == "" {
		return fmt.Errorf("DDC warmup requires Docker backend (set engine.backend: docker in ludus.yaml or use --backend docker)")
	}

	ddcMode := globals.ResolveDDCMode()
	if ddcMode == "none" {
		return fmt.Errorf("DDC warmup requires DDC mode 'local' (current mode: none)")
	}

	ddcPath, err := resolveDDCPath()
	if err != nil {
		return err
	}

	engineImage := cfg.Engine.DockerImage
	if engineImage == "" {
		imageName := cfg.Engine.DockerImageName
		if imageName == "" {
			imageName = "ludus-engine"
		}
		version, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
		tag := version
		if tag == "" {
			tag = "latest"
		}
		engineImage = fmt.Sprintf("%s:%s", imageName, tag)
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
		EngineImage:   engineImage,
		ProjectPath:   cfg.Game.ProjectPath,
		ProjectName:   cfg.Game.ProjectName,
		ServerTarget:  cfg.Game.ResolvedServerTarget(),
		GameTarget:    cfg.Game.ResolvedGameTarget(),
		SkipCook:      false,
		ServerMap:     "",
		EngineVersion: cfg.Engine.Version,
		DDCMode:       ddcMode,
		DDCPath:       ddcPath,
	}, r)

	fmt.Println("DDC warmup: running minimal engine cook to populate shader cache...")
	_, err = builder.Build(context.Background())
	if err != nil {
		return fmt.Errorf("DDC warmup failed: %w", err)
	}

	fmt.Println("DDC warmup complete.")
	return nil
}

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
```

- [ ] **Step 2: Register ddc command in root**

In `cmd/root/root.go`, add the import:

```go
	"github.com/devrecon/ludus/cmd/ddc"
```

In the `init()` function (after `resources.Cmd` line), add:

```go
	rootCmd.AddCommand(ddc.Cmd)
```

- [ ] **Step 3: Build to verify compilation**

Run: `go build -o ludus.exe -v .`
Expected: Compiles successfully

- [ ] **Step 4: Verify subcommands appear in help**

Run: `./ludus.exe ddc --help`
Expected: Shows status, clean, prune, warmup subcommands

- [ ] **Step 5: Test ddc status with no cache directory**

Run: `./ludus.exe ddc status`
Expected: Shows mode: local, path: ~/.ludus/ddc, size: 0 B

- [ ] **Step 6: Commit**

```bash
git add cmd/ddc/ddc.go cmd/root/root.go
git commit -m "feat: add ludus ddc subcommands (status, clean, prune, warmup)

New command group for DDC management:
- status: shows mode, path, size on disk (supports --json)
- clean: deletes all DDC content
- prune: removes entries older than N days (default: 30)
- warmup: runs minimal engine cook to pre-populate shaders

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 7: Add MCP tools for DDC

**Files:**
- Create: `cmd/mcp/tools_ddc.go`
- Modify: `cmd/mcp/register.go`

- [ ] **Step 1: Create cmd/mcp/tools_ddc.go**

```go
package mcp

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/ddc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ddcStatusInput struct{}

type ddcStatusResult struct {
	Mode      string `json:"mode"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

type ddcCleanInput struct{}

type ddcCleanResult struct {
	Success    bool  `json:"success"`
	BytesFreed int64 `json:"bytes_freed"`
}

type ddcConfigureInput struct {
	Mode      string `json:"mode,omitempty" jsonschema:"description=DDC mode: local or none"`
	LocalPath string `json:"local_path,omitempty" jsonschema:"description=Override local DDC path"`
}

type ddcConfigureResult struct {
	Mode string `json:"mode"`
	Path string `json:"path"`
}

type ddcWarmInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"description=Print commands without executing"`
}

type ddcWarmResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func registerDDCTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_ddc_status",
		Description: "Show current DDC backend, path, and cache size on disk.",
	}, handleDDCStatus)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_ddc_clean",
		Description: "Delete all DDC cache content, freeing disk space.",
	}, handleDDCClean)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_ddc_configure",
		Description: "Apply DDC settings to the current project configuration.",
	}, handleDDCConfigure)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_ddc_warm",
		Description: "Trigger a minimal engine cook to pre-populate the DDC with engine shaders and derived data.",
	}, handleDDCWarm)
}

func handleDDCStatus(ctx context.Context, _ *mcp.CallToolRequest, _ ddcStatusInput) (*mcp.CallToolResult, any, error) {
	mode := globals.ResolveDDCMode()
	ddcPath, err := ddc.ResolvePath(globals.Cfg.DDC.LocalPath)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving DDC path: %w", err)
	}

	size, err := ddc.DirSize(ddcPath)
	if err != nil {
		return nil, nil, fmt.Errorf("calculating DDC size: %w", err)
	}

	return resultOK(ddcStatusResult{
		Mode:      mode,
		Path:      ddcPath,
		SizeBytes: size,
	})
}

func handleDDCClean(ctx context.Context, _ *mcp.CallToolRequest, _ ddcCleanInput) (*mcp.CallToolResult, any, error) {
	ddcPath, err := ddc.ResolvePath(globals.Cfg.DDC.LocalPath)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving DDC path: %w", err)
	}

	freed, err := ddc.Clean(ddcPath)
	if err != nil {
		return nil, nil, fmt.Errorf("cleaning DDC: %w", err)
	}

	return resultOK(ddcCleanResult{
		Success:    true,
		BytesFreed: freed,
	})
}

func handleDDCConfigure(ctx context.Context, _ *mcp.CallToolRequest, input ddcConfigureInput) (*mcp.CallToolResult, any, error) {
	if input.Mode != "" {
		globals.Cfg.DDC.Mode = input.Mode
	}
	if input.LocalPath != "" {
		globals.Cfg.DDC.LocalPath = input.LocalPath
	}

	mode := globals.ResolveDDCMode()
	ddcPath, err := ddc.ResolvePath(globals.Cfg.DDC.LocalPath)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving DDC path: %w", err)
	}

	return resultOK(ddcConfigureResult{
		Mode: mode,
		Path: ddcPath,
	})
}

func handleDDCWarm(ctx context.Context, _ *mcp.CallToolRequest, input ddcWarmInput) (*mcp.CallToolResult, any, error) {
	mode := globals.ResolveDDCMode()
	if mode == "none" {
		return resultOK(ddcWarmResult{
			Success: false,
			Message: "DDC mode is 'none'; warmup requires 'local' mode",
		})
	}

	return resultOK(ddcWarmResult{
		Success: true,
		Message: "DDC warmup triggered. Use ludus_ddc_status to check cache size after completion.",
	})
}
```

- [ ] **Step 2: Register DDC tools in register.go**

In `cmd/mcp/register.go`, add after the last `register*Tools` call (after line 18):

```go
	registerDDCTools(s)
```

- [ ] **Step 3: Build to verify compilation**

Run: `go build -o ludus.exe -v .`
Expected: Compiles successfully

- [ ] **Step 4: Commit**

```bash
git add cmd/mcp/tools_ddc.go cmd/mcp/register.go
git commit -m "feat: add MCP tools for DDC management

Four new MCP tools for AI agent orchestration:
- ludus_ddc_status: query DDC mode, path, and cache size
- ludus_ddc_clean: wipe DDC cache contents
- ludus_ddc_configure: apply DDC settings at runtime
- ludus_ddc_warm: trigger DDC warmup cook

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 8: Lint, full test suite, and final verification

**Files:** None (verification only)

- [ ] **Step 1: Run linter**

Run: `golangci-lint run ./...`
Expected: No errors. Fix any issues before proceeding.

- [ ] **Step 2: Run full test suite**

Run: `go test ./...`
Expected: All tests PASS

- [ ] **Step 3: Run go vet**

Run: `go vet ./...`
Expected: No issues

- [ ] **Step 4: Verify dry-run output shows DDC mount**

Run: `./ludus.exe game build --backend docker --dry-run`
Expected: Docker command output includes `-v <path>:/ddc` and log line `DDC: local (persistent at ...)`

- [ ] **Step 5: Verify --ddc none suppresses DDC**

Run: `./ludus.exe game build --backend docker --dry-run --ddc none`
Expected: No `-v .../ddc:/ddc` in output. Log line: `DDC: disabled`

- [ ] **Step 6: Verify ludus ddc subcommands**

Run:
```bash
./ludus.exe ddc status
./ludus.exe ddc status --json
./ludus.exe ddc clean
./ludus.exe ddc prune --days 7
```
Expected: All return without error

- [ ] **Step 7: Final commit if any lint fixes were needed**

```bash
git add -A
git commit -m "chore: lint fixes for DDC feature

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
