package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/jpvelasco/ludus/internal/wsl"
)

func TestMakeGameBuildOptsWithDDC(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engine.SourcePath = "/engine"
	cfg.Game.ProjectPath = "/game/MyGame.uproject"
	cfg.Game.ProjectName = "MyGame"
	cfg.Game.ServerTarget = "MyServer"
	cfg.Game.ClientTarget = "MyClient"
	cfg.Game.GameTarget = "MyGame"
	cfg.Game.Platform = "Linux"
	cfg.Game.Arch = "arm64"
	cfg.Game.ServerMap = "/Game/Maps/Arena"

	got := makeGameBuildOptsWithDDC(
		cfg, true, "Win64", "Shipping", 12, "5.6", "local", "/ddc",
	)
	fields := map[string]struct {
		got  string
		want string
	}{
		"engine path":     {got.EnginePath, "/engine"},
		"project path":    {got.ProjectPath, "/game/MyGame.uproject"},
		"project name":    {got.ProjectName, "MyGame"},
		"server target":   {got.ServerTarget, "MyServer"},
		"client target":   {got.ClientTarget, "MyClient"},
		"game target":     {got.GameTarget, "MyGame"},
		"platform":        {got.Platform, "Linux"},
		"arch":            {got.Arch, "arm64"},
		"client platform": {got.ClientPlatform, "Win64"},
		"server map":      {got.ServerMap, "/Game/Maps/Arena"},
		"engine version":  {got.EngineVersion, "5.6"},
		"server config":   {got.ServerConfig, "Shipping"},
		"ddc mode":        {got.DDCMode, "local"},
		"ddc path":        {got.DDCPath, "/ddc"},
	}
	for name, field := range fields {
		if field.got != field.want {
			t.Errorf("%s = %q, want %q", name, field.got, field.want)
		}
	}
	if !got.SkipCook {
		t.Error("SkipCook = false, want true")
	}
	if got.MaxJobs != 12 {
		t.Errorf("MaxJobs = %d, want 12", got.MaxJobs)
	}
}

func TestResolveWSL2DDCPath(t *testing.T) {
	w := &wsl.WSL2{}

	tests := []struct {
		name        string
		engineState *state.WSL2EngineState
		mode        string
		want        string
	}{
		{
			name:        "state path wins",
			engineState: &state.WSL2EngineState{DDCPath: "~/ludus/ddc"},
			mode:        "local",
			want:        "~/ludus/ddc",
		},
		{
			name:        "non-local mode stays empty",
			engineState: &state.WSL2EngineState{},
			mode:        "zen",
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveWSL2DDCPath(w, tt.engineState, tt.mode, "C:\\ddc"); got != tt.want {
				t.Fatalf("resolveWSL2DDCPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWSLGameOptsPropagateConfig pins the shared WSL2 game option assembly used
// by both the sync and async ludus_game_build handlers: the archive root must
// be resolved (#751 — an empty -archivedirectory sends the packaged build to
// an unknown location) and the merged skip-cook config must flow through.
func TestWSLGameOptsPropagateConfig(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: "/engine"},
		Game: config.GameConfig{
			ProjectName:  "TestGame",
			ProjectPath:  "/game/TestGame.uproject",
			ServerTarget: "TestGameServer",
			SkipCook:     true,
		},
	}
	engineState := &state.WSL2EngineState{EnginePath: "/home/user/engine"}

	opts := wslGameOpts(cfg, engineState, &wsl.WSL2{}, ddc.ModeNone, "", "Shipping", 8)

	if opts.OutputDir != config.ResolveServerArchiveRoot(cfg) {
		t.Errorf("wslGameOpts().OutputDir = %q, want archive root %q", opts.OutputDir, config.ResolveServerArchiveRoot(cfg))
	}
	if !opts.SkipCook {
		t.Error("wslGameOpts().SkipCook = false, want true (from merged config)")
	}
	fields := map[string]struct{ got, want string }{
		"ServerConfig": {opts.ServerConfig, "Shipping"},
		"ServerTarget": {opts.ServerTarget, "TestGameServer"},
		"EnginePath":   {opts.EnginePath, "/home/user/engine"},
		"DDCMode":      {opts.DDCMode, ddc.ModeNone},
	}
	for name, f := range fields {
		if f.got != f.want {
			t.Errorf("wslGameOpts().%s = %q, want %q", name, f.got, f.want)
		}
	}
	if opts.MaxJobs != 8 {
		t.Errorf("wslGameOpts().MaxJobs = %d, want 8", opts.MaxJobs)
	}
}

// TestHandleContainerGameClientArchivesToClientRoot pins #750 for MCP: a
// container client build must archive under the PackagedClient root, not the
// PackagedServer root. The dry-run build reports the resolved output directory
// in the tool result.
func TestHandleContainerGameClientArchivesToClientRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "/engine",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "docker",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: testsupport.FakeProject(t, "TestGame"),
			Platform:    "Linux",
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDDCMode("none"))

	result, _, err := handleGameClient(context.Background(), nil, gameClientInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleGameClient() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handleGameClient() returned error result: %s", toolResultText(t, result))
	}

	var build gameBuildResult
	if err := json.Unmarshal([]byte(toolResultText(t, result)), &build); err != nil {
		t.Fatalf("unmarshalling result: %v", err)
	}

	clientRoot := config.ResolveClientArchiveRoot(cfg)
	if build.OutputDir != clientRoot {
		t.Errorf("OutputDir = %q, want the PackagedClient root %q", build.OutputDir, clientRoot)
	}
	if serverRoot := config.ResolveServerArchiveRoot(cfg); build.OutputDir == serverRoot {
		t.Errorf("OutputDir %q must not be the PackagedServer root", build.OutputDir)
	}
}
