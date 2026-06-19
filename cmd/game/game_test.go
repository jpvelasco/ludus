package game

import (
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/wsl"
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
