package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/spf13/cobra"
)

func engineCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

// seedEngineCache chdirs to a temp dir and writes a cache entry marking the
// engine stage up to date for the given hash.
func seedEngineCache(t *testing.T, hash string) {
	t.Helper()
	t.Chdir(t.TempDir())
	c := &cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
		cache.StageEngine: {Hash: hash, BuiltAt: "test"},
	}}
	if err := cache.Save(c); err != nil {
		t.Fatal(err)
	}
}

// TestRunBuild_PrereqFailure covers the CheckEngineReady error branch of
// runBuild: an empty engine source path fails before any dispatch.
func TestRunBuild_PrereqFailure(t *testing.T) {
	globals.SetGlobals(t, &config.Config{}, globals.WithDryRun(true))

	err := runBuild(engineCmd(t), nil)
	if err == nil || !strings.Contains(err.Error(), "prerequisite check(s) failed") {
		t.Fatalf("runBuild() error = %v, want prereq failure", err)
	}
}

// TestRunSetup_MissingSourcePath covers the makeBuilder error branch of
// runSetup.
func TestRunSetup_MissingSourcePath(t *testing.T) {
	globals.SetGlobals(t, &config.Config{})

	err := runSetup(engineCmd(t), nil)
	if err == nil || !strings.Contains(err.Error(), "engine source path not configured") {
		t.Fatalf("runSetup() error = %v, want source path error", err)
	}
}

// TestRunSetup_Success covers the full success path of runSetup, including
// the maybeRunMacOSPreflights early return for non-macOS hosts.
func TestRunSetup_Success(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
		},
		Game: config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	if err := runSetup(engineCmd(t), nil); err != nil {
		t.Fatalf("runSetup() error = %v, want nil", err)
	}
}

// TestRunNativeEngineBuild_CacheHit covers the cache-skip early return of
// runNativeEngineBuild.
func TestRunNativeEngineBuild_CacheHit(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: "C:/ue5", Version: "5.7.3"},
		Game:   config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	seedEngineCache(t, cache.EngineKey(cfg))

	if err := runNativeEngineBuild(engineCmd(t)); err != nil {
		t.Fatalf("runNativeEngineBuild(cached) error = %v, want nil", err)
	}
}

// TestRunContainerBuild_CacheHit covers the cache-skip early return of
// runContainerBuild.
func TestRunContainerBuild_CacheHit(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "C:/ue5",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "docker",
		},
		Game: config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	seedEngineCache(t, cache.EngineKey(cfg))

	if err := runContainerBuild(engineCmd(t), "docker"); err != nil {
		t.Fatalf("runContainerBuild(cached) error = %v, want nil", err)
	}
}

// TestRunContainerBuild_MissingSourcePath covers the makeContainerEngineBuilder
// error branch of runContainerBuild.
func TestRunContainerBuild_MissingSourcePath(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "docker",
		},
		Game: config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	t.Chdir(t.TempDir())

	err := runContainerBuild(engineCmd(t), "docker")
	if err == nil || !strings.Contains(err.Error(), "engine source path not configured") {
		t.Fatalf("runContainerBuild() error = %v, want source path error", err)
	}
}

// TestRunContainerBuild_SkipEnginePackaging covers the skip-engine packaging
// branch of runContainerBuild: with --skip-engine and no cache, the engine
// binaries are packaged instead of built.
func TestRunContainerBuild_SkipEnginePackaging(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	binDir := filepath.Join(engineRoot, "Engine", "Binaries", "Linux")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "UnrealEditor"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  engineRoot,
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "docker",
		},
		Game: config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	t.Chdir(t.TempDir())

	skipEngine, noCache = true, true
	t.Cleanup(func() { skipEngine, noCache = false, false })

	output := captureStdout(func() {
		if err := runContainerBuild(engineCmd(t), "docker"); err != nil {
			t.Fatalf("runContainerBuild() error = %v, want nil", err)
		}
	})
	if !strings.Contains(output, "Packaging pre-built engine binaries") {
		t.Fatalf("runContainerBuild() output missing skip-engine line: %s", output)
	}
}

// TestRunContainerBuild_StateWarning covers the UpdateEngineImage failure
// branch of runContainerBuild: a write failure degrades to a warning.
func TestRunContainerBuild_StateWarning(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  engineRoot,
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "docker",
		},
		Game: config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, ".ludus"), []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}
	noCache = true
	t.Cleanup(func() { noCache = false })

	output := captureStdout(func() {
		if err := runContainerBuild(engineCmd(t), "docker"); err != nil {
			t.Fatalf("runContainerBuild() error = %v, want nil with warning", err)
		}
	})
	if !strings.Contains(output, "Warning: failed to write state") {
		t.Fatalf("runContainerBuild() output missing state warning: %s", output)
	}
}

// TestRunWSL2Build_MissingSourcePath covers the missing source path branch of
// runWSL2Build before any wsl.exe probing.
func TestRunWSL2Build_MissingSourcePath(t *testing.T) {
	globals.SetGlobals(t, &config.Config{}, globals.WithDryRun(true))

	err := runWSL2Build(engineCmd(t))
	if err == nil || !strings.Contains(err.Error(), "engine source path not configured") {
		t.Fatalf("runWSL2Build() error = %v, want source path error", err)
	}
}

// TestRunWSL2Build_WslUnavailable covers the wsl.New error branch of
// runWSL2Build: a configured source path but no wsl.exe on PATH.
func TestRunWSL2Build_WslUnavailable(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			Backend:    "wsl2",
		},
		Game: config.GameConfig{ProjectName: "TestGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	t.Setenv("PATH", t.TempDir())

	err := runWSL2Build(engineCmd(t))
	if err == nil || !strings.Contains(err.Error(), "WSL2 is not available") {
		t.Fatalf("runWSL2Build() error = %v, want WSL2 unavailable error", err)
	}
}
