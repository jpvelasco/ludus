package game

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/spf13/cobra"
)

// seedGameCache chdirs to a temp dir and writes a cache entry that marks the
// given stage up to date for the given hash.
func seedGameCache(t *testing.T, stage cache.StageKey, hash string) {
	t.Helper()
	t.Chdir(t.TempDir())
	c := &cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
		stage: {Hash: hash, BuiltAt: "test"},
	}}
	if err := cache.Save(c); err != nil {
		t.Fatal(err)
	}
}

func gameCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

// TestRunBuild_PrereqFailure covers the Validate error branch of runBuild:
// an empty engine source path fails CheckGameReady before any dispatch.
func TestRunBuild_PrereqFailure(t *testing.T) {
	globals.SetGlobals(t, &config.Config{}, globals.WithDryRun(true))

	err := runBuild(gameCmd(t), nil)
	if err == nil || !strings.Contains(err.Error(), "prerequisite check(s) failed") {
		t.Fatalf("runBuild() error = %v, want prereq failure", err)
	}
}

// TestRunBuild_WSL2CacheHit covers the WSL2 dispatch branch of runBuild plus
// the cache-hit early return inside runWSL2GameBuild.
func TestRunBuild_WSL2CacheHit(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			Backend:    "wsl2",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Platform:    "Linux",
			Arch:        "amd64",
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	resolved := resolvedBuildConfig()
	engineHash := cache.EngineKey(&resolved)
	serverHash := cache.GameServerKey(&resolved, engineHash)
	seedGameCache(t, cache.StageGameServer, serverHash)

	if err := runBuild(gameCmd(t), nil); err != nil {
		t.Fatalf("runBuild(wsl2, cached) error = %v, want nil", err)
	}
}

// TestRunContainerBuild_PreflightFailsWithoutDryRun covers the non-dry-run
// preflight branch: a failing docker daemon hard-fails the check before any
// build work happens.
func TestRunContainerBuild_PreflightFailsWithoutDryRun(t *testing.T) {
	projectPath := testsupport.FakeProject(t, "TestGame")
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "C:/ue5",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "docker",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
	}
	globals.SetGlobals(t, cfg)
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 1})

	err := runContainerBuild(gameCmd(t), "docker", cfg)
	if err == nil || !strings.Contains(err.Error(), "prerequisite check(s) failed") {
		t.Fatalf("runContainerBuild() error = %v, want preflight failure", err)
	}
}

// TestRunContainerBuild_CacheHit covers the cache-hit early return of
// runContainerBuild after the (dry-run-skipped) preflight.
func TestRunContainerBuild_CacheHit(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "C:/ue5",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "docker",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "C:/project/TestGame.uproject",
			Platform:    "Linux",
			Arch:        "amd64",
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	serverHash := cache.GameServerKey(cfg, cache.EngineKey(cfg))
	seedGameCache(t, cache.StageGameServer, serverHash)

	if err := runContainerBuild(gameCmd(t), "docker", cfg); err != nil {
		t.Fatalf("runContainerBuild(cached) error = %v, want nil", err)
	}
}

// TestRunContainerBuild_InvalidDDCMode covers the ResolveContainerGameOptions
// error branch of runContainerBuild.
func TestRunContainerBuild_InvalidDDCMode(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "C:/ue5",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "docker",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "C:/project/TestGame.uproject",
		},
		DDC: config.DDCConfig{Mode: "bogus"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	t.Chdir(t.TempDir())

	err := runContainerBuild(gameCmd(t), "docker", cfg)
	if err == nil || !strings.Contains(err.Error(), "resolving DDC config") {
		t.Fatalf("runContainerBuild() error = %v, want DDC config error", err)
	}
}

// TestRunClientBuild_ContainerDispatch covers the container dispatch branch of
// runClientBuild: a podman backend routes to the container client build.
func TestRunClientBuild_ContainerDispatch(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "C:/ue5",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "native",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "C:/project/TestGame.uproject",
			Platform:    "Linux",
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	origBackend, origPlatform := backend, clientPlatform
	backend, clientPlatform = "podman", "Linux"
	t.Cleanup(func() { backend, clientPlatform = origBackend, origPlatform })

	if err := runClientBuild(gameCmd(t), nil); err != nil {
		t.Fatalf("runClientBuild(container dispatch) error = %v, want nil", err)
	}
}

// TestRunClientBuild_CacheHit covers the cache-hit early return of
// runClientBuild before any engine path resolution.
func TestRunClientBuild_CacheHit(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "C:/ue5",
			Version:    "5.7.3",
			Backend:    "native",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "C:/project/TestGame.uproject",
			Arch:        "amd64",
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	origPlatform := clientPlatform
	clientPlatform = "Linux"
	t.Cleanup(func() { clientPlatform = origPlatform })

	clientHash := cache.GameClientKey(cfg, cache.EngineKey(cfg), clientPlatform)
	seedGameCache(t, cache.StageGameClient, clientHash)

	if err := runClientBuild(gameCmd(t), nil); err != nil {
		t.Fatalf("runClientBuild(cached) error = %v, want nil", err)
	}
}

// TestRunContainerClientBuild_CacheHit covers the cache-hit early return of
// runContainerClientBuild.
func TestRunContainerClientBuild_CacheHit(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "C:/ue5",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "docker",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "C:/project/TestGame.uproject",
			Arch:        "amd64",
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	origPlatform := clientPlatform
	clientPlatform = "Linux"
	t.Cleanup(func() { clientPlatform = origPlatform })

	clientHash := cache.GameClientKey(cfg, cache.EngineKey(cfg), clientPlatform)
	seedGameCache(t, cache.StageGameClient, clientHash)

	if err := runContainerClientBuild(gameCmd(t), "docker"); err != nil {
		t.Fatalf("runContainerClientBuild(cached) error = %v, want nil", err)
	}
}

// TestRunContainerClientBuild_InvalidDDCMode covers the
// ResolveContainerGameOptions error branch of runContainerClientBuild.
func TestRunContainerClientBuild_InvalidDDCMode(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "C:/ue5",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "docker",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "C:/project/TestGame.uproject",
		},
		DDC: config.DDCConfig{Mode: "bogus"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	t.Chdir(t.TempDir())

	err := runContainerClientBuild(gameCmd(t), "docker")
	if err == nil || !strings.Contains(err.Error(), "resolving DDC config") {
		t.Fatalf("runContainerClientBuild() error = %v, want DDC config error", err)
	}
}

// TestRunNativeBuild_InvalidDDCMode covers the ResolveDDC error branch of
// runNativeBuild.
func TestRunNativeBuild_InvalidDDCMode(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			Backend:    "native",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
		DDC: config.DDCConfig{Mode: "bogus"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	t.Chdir(t.TempDir())

	err := runNativeBuild(gameCmd(t), cfg, "test_hash")
	if err == nil || !strings.Contains(err.Error(), "invalid DDC mode") {
		t.Fatalf("runNativeBuild() error = %v, want invalid DDC mode error", err)
	}
}

// TestRunBuild_NativeCacheHit covers the cache-skip early return at the
// runBuild level for the native backend.
func TestRunBuild_NativeCacheHit(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			Backend:    "native",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Arch:        "amd64",
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	serverHash := cache.GameServerKey(cfg, cache.EngineKey(cfg))
	seedGameCache(t, cache.StageGameServer, serverHash)

	if err := runBuild(gameCmd(t), nil); err != nil {
		t.Fatalf("runBuild(native, cached) error = %v, want nil", err)
	}
}

// TestRunNativeBuild_MissingSourcePath covers the missing engine source path
// branch of runNativeBuild.
func TestRunNativeBuild_MissingSourcePath(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: "", Version: "5.7.3"},
		Game:   config.GameConfig{ProjectName: "TestGame", ProjectPath: "C:/project/TestGame.uproject"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	t.Chdir(t.TempDir())

	err := runNativeBuild(gameCmd(t), cfg, "test_hash")
	if err == nil || !strings.Contains(err.Error(), "engine source path not configured") {
		t.Fatalf("runNativeBuild() error = %v, want source path error", err)
	}
}

// TestRunWSL2GameBuild_CorruptState covers the state load error branch of
// runWSL2GameBuild: an unparseable state file fails the build before any WSL2
// probing.
func TestRunWSL2GameBuild_CorruptState(t *testing.T) {
	t.Chdir(t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })

	if err := os.MkdirAll(".ludus", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".ludus", "state.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: "C:/ue5", Version: "5.7.3", Backend: "wsl2"},
		Game:   config.GameConfig{ProjectName: "TestGame", ProjectPath: "C:/project/TestGame.uproject"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	err := runWSL2GameBuild(gameCmd(t), cfg)
	if err == nil || !strings.Contains(err.Error(), "loading state") {
		t.Fatalf("runWSL2GameBuild() error = %v, want state load error", err)
	}
}

// TestRunWSL2GameBuild_WslUnavailable covers the wsl.New error branch of
// runWSL2GameBuild: valid engine state but no wsl.exe on PATH.
func TestRunWSL2GameBuild_WslUnavailable(t *testing.T) {
	t.Chdir(t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })

	if err := state.UpdateWSL2Engine(&state.WSL2EngineState{
		EnginePath: "/home/user/ludus/engine/5.7",
		DDCPath:    "/home/user/ludus/ddc",
		IsNative:   true,
	}); err != nil {
		t.Fatalf("UpdateWSL2Engine() error = %v", err)
	}

	t.Setenv("PATH", t.TempDir())

	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: "C:/ue5", Version: "5.7.3", Backend: "wsl2"},
		Game:   config.GameConfig{ProjectName: "TestGame", ProjectPath: "C:/project/TestGame.uproject"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	err := runWSL2GameBuild(gameCmd(t), cfg)
	if err == nil || !strings.Contains(err.Error(), "WSL2 is not available") {
		t.Fatalf("runWSL2GameBuild() error = %v, want WSL2 unavailable error", err)
	}
}

// TestRunWSL2GameBuild_InvalidDDCMode covers the ResolveDDC error branch of
// runWSL2GameBuild: the wsl.exe stub answers the distro probe, then DDC
// resolution fails on an invalid mode.
func TestRunWSL2GameBuild_InvalidDDCMode(t *testing.T) {
	t.Chdir(t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })

	if err := state.UpdateWSL2Engine(&state.WSL2EngineState{
		EnginePath: "/home/user/ludus/engine/5.7",
		DDCPath:    "/home/user/ludus/ddc",
		IsNative:   true,
	}); err != nil {
		t.Fatalf("UpdateWSL2Engine() error = %v", err)
	}

	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{
		Stdout: "* Ubuntu          Running         2",
	})

	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: "C:/ue5", Version: "5.7.3", Backend: "wsl2"},
		Game:   config.GameConfig{ProjectName: "TestGame", ProjectPath: "C:/project/TestGame.uproject"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithDDCMode("bogus"))

	err := runWSL2GameBuild(gameCmd(t), cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid DDC mode") {
		t.Fatalf("runWSL2GameBuild() error = %v, want invalid DDC mode error", err)
	}
}
