package game

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/jpvelasco/ludus/internal/wsl"
	"github.com/spf13/cobra"
)

// makeTestWSL2 constructs a minimal WSL2 coordinator suitable for path tests.
// Does not call wsl.New so it does not require a live WSL2 environment.
func makeTestWSL2() *wsl.WSL2 {
	return &wsl.WSL2{Distro: "Ubuntu"}
}

func TestBuildWSL2GameOptions_FieldMapping(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })

	cfg := &config.Config{}
	cfg.Game.ProjectPath = `F:\Projects\MyGame\MyGame.uproject`
	cfg.Game.ProjectName = "MyGame"
	cfg.Game.ServerTarget = "MyGameServer"
	cfg.Game.Platform = "Linux"
	cfg.Game.Arch = "amd64"
	cfg.Game.ServerMap = "/Game/Maps/ServerMap"
	globals.Cfg = cfg

	s := &state.State{
		WSL2Engine: &state.WSL2EngineState{
			EnginePath: "/home/user/ludus/engine/5.5",
			DDCPath:    "/home/user/ludus/ddc",
		},
	}

	w := makeTestWSL2()

	opts := buildWSL2GameOptions(cfg, s, w, ddc.ModeLocal, "/home/user/ludus/ddc")

	if opts.EnginePath != s.WSL2Engine.EnginePath {
		t.Errorf("EnginePath = %q, want %q", opts.EnginePath, s.WSL2Engine.EnginePath)
	}
	if opts.ProjectPath != cfg.Game.ProjectPath {
		t.Errorf("ProjectPath = %q, want %q", opts.ProjectPath, cfg.Game.ProjectPath)
	}
	if opts.ProjectName != "MyGame" {
		t.Errorf("ProjectName = %q, want %q", opts.ProjectName, "MyGame")
	}
	if opts.ServerTarget != cfg.Game.ResolvedServerTarget() {
		t.Errorf("ServerTarget = %q, want %q", opts.ServerTarget, cfg.Game.ResolvedServerTarget())
	}
	if opts.Platform != "Linux" {
		t.Errorf("Platform = %q, want %q", opts.Platform, "Linux")
	}
	if opts.ServerMap != "/Game/Maps/ServerMap" {
		t.Errorf("ServerMap = %q, want %q", opts.ServerMap, "/Game/Maps/ServerMap")
	}
}

func TestResolvedBuildConfigAppliesArchOverrideWithoutMutatingGlobal(t *testing.T) {
	origCfg := globals.Cfg
	origArchFlag := archFlag
	t.Cleanup(func() {
		globals.Cfg = origCfg
		archFlag = origArchFlag
	})

	globals.Cfg = &config.Config{}
	globals.Cfg.Game.Arch = "amd64"
	archFlag = "arm64"

	cfg := resolvedBuildConfig()
	if got := cfg.Game.ResolvedArch(); got != "arm64" {
		t.Errorf("resolved build arch = %q, want arm64", got)
	}
	if got := globals.Cfg.Game.ResolvedArch(); got != "amd64" {
		t.Errorf("global config arch = %q, want amd64", got)
	}

	overrideKey := cache.GameServerKey(&cfg, cache.EngineKey(&cfg))
	globalKey := cache.GameServerKey(globals.Cfg, cache.EngineKey(globals.Cfg))
	if overrideKey == globalKey {
		t.Error("cache key should change when --arch overrides configured architecture")
	}
}

// TestResolvedBuildConfigMergesSkipCook pins the #558 contract: the CLI
// --skip-cook flag must land in the resolved config so cache-key hashing and
// build behavior share one source of truth.
func TestResolvedBuildConfigMergesSkipCook(t *testing.T) {
	origCfg := globals.Cfg
	origSkip := skipCook
	t.Cleanup(func() {
		globals.Cfg = origCfg
		skipCook = origSkip
	})

	globals.Cfg = &config.Config{}
	skipCook = true

	cfg := resolvedBuildConfig()
	if !cfg.Game.SkipCook {
		t.Error("resolved config SkipCook = false with --skip-cook set; cache keys and behavior diverge")
	}
}

func TestBuildWSL2GameOptions_OutputDirSet(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })

	cfg := &config.Config{}
	cfg.Game.ProjectPath = `F:\Projects\MyGame\MyGame.uproject`
	cfg.Game.ProjectName = "MyGame"
	globals.Cfg = cfg

	s := &state.State{
		WSL2Engine: &state.WSL2EngineState{EnginePath: "/mnt/f/engine"},
	}

	opts := buildWSL2GameOptions(cfg, s, makeTestWSL2(), ddc.ModeNone, "")

	// OutputDir must be non-empty (resolved from project path)
	if opts.OutputDir == "" {
		t.Error("OutputDir should be non-empty")
	}
}

func TestResolveWSL2GameDDCPath_LocalModeNoEnginePathConvertsToWSL(t *testing.T) {
	w := makeTestWSL2()
	// Local mode + no engine DDC path → convert the ddcPath to WSL
	got := resolveWSL2GameDDCPath(w, "", ddc.ModeLocal, `C:\Users\user\.ludus\ddc`)
	want := "/mnt/c/Users/user/.ludus/ddc"
	if got != want {
		t.Errorf("resolveWSL2GameDDCPath = %q, want %q", got, want)
	}
}

func TestResolveWSL2GameDDCPath_LocalModeWithEnginePathUsesEnginePath(t *testing.T) {
	w := makeTestWSL2()
	engineDDCPath := "/home/user/ludus/ddc"
	got := resolveWSL2GameDDCPath(w, engineDDCPath, ddc.ModeLocal, `C:\Users\user\.ludus\ddc`)
	if got != engineDDCPath {
		t.Errorf("resolveWSL2GameDDCPath = %q, want %q", got, engineDDCPath)
	}
}

func TestResolveWSL2GameDDCPath_NonLocalModeReturnsEnginePath(t *testing.T) {
	w := makeTestWSL2()
	engineDDCPath := "/home/user/ludus/ddc"
	got := resolveWSL2GameDDCPath(w, engineDDCPath, ddc.ModeNone, "")
	if got != engineDDCPath {
		t.Errorf("resolveWSL2GameDDCPath = %q, want %q", got, engineDDCPath)
	}
}

func TestResolveWSL2GameDDCPath_NonLocalModeNoEnginePath(t *testing.T) {
	w := makeTestWSL2()
	got := resolveWSL2GameDDCPath(w, "", ddc.ModeNone, "")
	if got != "" {
		t.Errorf("resolveWSL2GameDDCPath = %q, want empty", got)
	}
}

// TestRunBuildNativeBackend tests native game build dispatch.
func TestRunBuildNativeBackend(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
			Backend:    "native",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runBuild(cmd, nil)
	if err != nil {
		t.Errorf("runBuild(native) error = %v, want nil", err)
	}
}

// TestRunBuildContainerBackend tests container game build dispatch.
func TestRunBuildContainerBackend(t *testing.T) {
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

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runBuild(cmd, nil)
	if err != nil {
		t.Errorf("runBuild(container) error = %v, want nil", err)
	}
}

// TestRunNativeBuild tests native game build with valid config.
func TestRunNativeBuild(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
			Backend:    "native",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	output := captureGameStdout(t, func() {
		err := runNativeBuild(cmd, cfg, "test_hash")
		if err != nil {
			t.Fatalf("runNativeBuild() error = %v, want nil", err)
		}
	})

	// Assert that RunUAT BuildCookRun is invoked
	if !strings.Contains(output, "RunUAT") {
		t.Errorf("output missing 'RunUAT' command: %s", output)
	}
	if !strings.Contains(output, "BuildCookRun") {
		t.Errorf("output missing 'BuildCookRun' command: %s", output)
	}
	if !strings.Contains(output, "Linux") {
		t.Errorf("output missing 'Linux' platform: %s", output)
	}
}

// TestRunNativeBuildMissingEnginePath tests native build with missing engine path.
func TestRunNativeBuildMissingEnginePath(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "",
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runNativeBuild(cmd, cfg, "test_hash")
	if err == nil {
		t.Fatal("runNativeBuild() error = nil, want error for missing engine path")
	}
	if !strings.Contains(err.Error(), "engine source path not configured") {
		t.Errorf("runNativeBuild() error = %v, want 'engine source path not configured'", err)
	}
}

// TestRunContainerBuild tests container game build with docker.
func TestRunContainerBuild(t *testing.T) {
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
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	output := captureGameStdout(t, func() {
		err := runContainerBuild(cmd, "docker", cfg)
		if err != nil {
			t.Fatalf("runContainerBuild() error = %v, want nil", err)
		}
	})

	// Assert that docker run command is produced (check for run subcommand)
	if !strings.Contains(output, "run") || (!strings.Contains(output, "docker") && !strings.Contains(output, "podman")) {
		t.Errorf("output missing 'docker/podman run' command: %s", output)
	}
	if !strings.Contains(output, "my.repo/engine:5.7.3") {
		t.Errorf("output missing engine image reference: %s", output)
	}
}

// TestRunClientBuild tests client build dispatch with native backend.
func TestRunClientBuild(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
			Backend:    "native",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	clientPlatform = "Linux"
	t.Cleanup(func() { clientPlatform = "Linux" })

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	output := captureGameStdout(t, func() {
		err := runClientBuild(cmd, nil)
		if err != nil {
			t.Fatalf("runClientBuild() error = %v, want nil", err)
		}
	})

	// Assert that RunUAT is invoked for client build
	if !strings.Contains(output, "RunUAT") {
		t.Errorf("output missing 'RunUAT' command: %s", output)
	}
}

// TestRunClientBuildMissingEnginePath tests client build with missing engine path.
func TestRunClientBuildMissingEnginePath(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "",
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runClientBuild(cmd, nil)
	if err == nil {
		t.Fatal("runClientBuild() error = nil, want error for missing engine path")
	}
	if !strings.Contains(err.Error(), "engine source path not configured") {
		t.Errorf("runClientBuild() error = %v, want 'engine source path not configured'", err)
	}
}

// TestRunContainerClientBuild tests container client build with docker.
func TestRunContainerClientBuild(t *testing.T) {
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
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	clientPlatform = "Linux"
	t.Cleanup(func() { clientPlatform = "Linux" })

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	output := captureGameStdout(t, func() {
		err := runContainerClientBuild(cmd, "docker")
		if err != nil {
			t.Fatalf("runContainerClientBuild() error = %v, want nil", err)
		}
	})

	// Assert that docker run command is produced for client build (check for run subcommand)
	if !strings.Contains(output, "run") || (!strings.Contains(output, "docker") && !strings.Contains(output, "podman")) {
		t.Errorf("output missing 'docker/podman run' command: %s", output)
	}
	if !strings.Contains(output, "my.repo/engine:5.7.3") {
		t.Errorf("output missing engine image reference: %s", output)
	}
}

// TestRunWSL2GameBuildRequiresEngineState verifies
// runWSL2GameBuild requires prior WSL2 engine state.
func TestRunWSL2GameBuildRequiresEngineState(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "C:/ue5",
			Version:    "5.7.3",
			MaxJobs:    1,
			Backend:    "wsl2",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "C:/project/TestGame.uproject",
			Platform:    "Linux",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	// Ensure state file doesn't exist in test env
	t.Chdir(t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	// WSL2 build should fail without prior engine build
	err := runWSL2GameBuild(cmd, cfg)
	if err == nil {
		t.Fatal("runWSL2GameBuild() error = nil, want error for missing WSL2 engine state")
	}
	if !strings.Contains(err.Error(), "no WSL2 engine build found") {
		t.Errorf("runWSL2GameBuild() error = %v, want 'no WSL2 engine build found'", err)
	}
}

// TestRunWSL2GameBuildMissingEngineState tests WSL2 build with missing engine state.
func TestRunWSL2GameBuildMissingEngineState(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "C:/ue5",
			Version:    "5.7.3",
			MaxJobs:    1,
			Backend:    "wsl2",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "C:/project/TestGame.uproject",
			Platform:    "Linux",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	// Ensure state file doesn't exist in test env
	t.Chdir(t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runWSL2GameBuild(cmd, cfg)
	if err == nil {
		t.Fatal("runWSL2GameBuild() error = nil, want error for missing WSL2 engine state")
	}
	if !strings.Contains(err.Error(), "no WSL2 engine build found") {
		t.Errorf("runWSL2GameBuild() error = %v, want 'no WSL2 engine build found'", err)
	}
}

// TestResolveGameBackendFlagOverridesConfig tests that CLI backend flag overrides config.
func TestResolveGameBackendFlagOverridesConfig(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			Backend: "docker",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	backend = "podman"
	t.Cleanup(func() { backend = "" })

	result := resolveBackend()
	if result != "podman" {
		t.Errorf("resolveBackend() = %q, want %q", result, "podman")
	}
}

// TestResolveArchFlagOverridesConfig tests that CLI arch flag overrides config.
func TestResolveArchFlagOverridesConfig(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			Arch: "amd64",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	archFlag = "aarch64"
	t.Cleanup(func() { archFlag = "" })

	result := resolveArch()
	if result != "arm64" {
		t.Errorf("resolveArch() = %q, want %q", result, "arm64")
	}
}

// TestResolveArchNormalizes tests arch normalization (aarch64 -> arm64).
func TestResolveArchNormalizes(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			Arch: "x86_64",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	archFlag = ""

	result := resolveArch()
	if result != "amd64" {
		t.Errorf("resolveArch() with x86_64 = %q, want amd64", result)
	}
}
