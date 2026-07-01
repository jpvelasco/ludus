package mcp

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
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
