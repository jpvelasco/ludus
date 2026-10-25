package globals

import (
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
)

// containerTestConfig returns a config with an external project so archive
// roots and target resolution are deterministic in tests.
func containerTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Game.ProjectPath = "/games/MyGame/MyGame.uproject"
	cfg.Game.ProjectName = "MyGame"
	cfg.Game.ServerTarget = "MyGameServer"
	cfg.Game.ClientTarget = "MyGameGame"
	cfg.Game.GameTarget = "MyGameGame"
	cfg.Game.ServerMap = "/Game/Maps/Arena"
	return cfg
}

func TestBaseDockerGameOptionsArch(t *testing.T) {
	tests := []struct {
		name string
		arch string
		want string
	}{
		{name: "arm64 config populates Arch", arch: "arm64", want: "arm64"},
		{name: "aarch64 alias normalizes to arm64", arch: "aarch64", want: "arm64"},
		{name: "x86_64 alias normalizes to amd64", arch: "x86_64", want: "amd64"},
		{name: "empty defaults to amd64", arch: "", want: "amd64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := containerTestConfig()
			cfg.Game.Arch = tt.arch

			opts := BaseDockerGameOptions(cfg, "ludus-engine:5.7", "5.7", "none", "", "", "docker")

			if opts.Arch != tt.want {
				t.Errorf("BaseDockerGameOptions().Arch = %q, want %q", opts.Arch, tt.want)
			}
		})
	}
}

func TestApplyServerBuildOptions(t *testing.T) {
	cfg := containerTestConfig()
	cfg.Game.SkipCook = true

	base := BaseDockerGameOptions(cfg, "ludus-engine:5.7", "5.7", "none", "", "", "docker")
	got := ApplyServerBuildOptions(base, cfg, "Shipping")

	fields := map[string]struct{ got, want string }{
		"ServerTarget": {got.ServerTarget, "MyGameServer"},
		"GameTarget":   {got.GameTarget, "MyGameGame"},
		"ServerMap":    {got.ServerMap, "/Game/Maps/Arena"},
		"ServerConfig": {got.ServerConfig, "Shipping"},
		"Arch":         {got.Arch, "amd64"},
		"OutputDir":    {got.OutputDir, config.ResolveServerArchiveRoot(cfg)},
	}
	for name, f := range fields {
		if f.got != f.want {
			t.Errorf("ApplyServerBuildOptions().%s = %q, want %q", name, f.got, f.want)
		}
	}
	if !got.SkipCook {
		t.Error("ApplyServerBuildOptions().SkipCook = false, want true")
	}
}

func TestApplyClientBuildOptions(t *testing.T) {
	cfg := containerTestConfig()
	cfg.Game.SkipCook = true

	base := BaseDockerGameOptions(cfg, "ludus-engine:5.7", "5.7", "none", "", "", "docker")
	got := ApplyClientBuildOptions(base, cfg, "Linux")

	fields := map[string]struct{ got, want string }{
		"ClientTarget":    {got.ClientTarget, "MyGameGame"},
		"ClientPlatform":  {got.ClientPlatform, "Linux"},
		"OutputDir":       {got.OutputDir, config.ResolveClientArchiveRoot(cfg)},
		"PackagedClient":  {got.OutputDir, filepath.Join("/games/MyGame", "PackagedClient")},
		"NotServerRoot":   {got.OutputDir, config.ResolveServerArchiveRoot(cfg)},
		"PreservedCommon": {got.EngineImage, "ludus-engine:5.7"},
	}
	for name, f := range fields {
		if name == "NotServerRoot" {
			if f.got == f.want {
				t.Errorf("client OutputDir %q must not be the server archive root", f.got)
			}
			continue
		}
		if f.got != f.want {
			t.Errorf("ApplyClientBuildOptions().%s = %q, want %q", name, f.got, f.want)
		}
	}
	if !got.SkipCook {
		t.Error("ApplyClientBuildOptions().SkipCook = false, want true")
	}
}

func TestResolveContainerBuildOptions(t *testing.T) {
	t.Chdir(t.TempDir())
	origCfg, origMode := Cfg, DDCMode
	t.Cleanup(func() { Cfg, DDCMode = origCfg, origMode })
	DDCMode = "none"

	tests := []struct {
		name string
		call func(cfg *config.Config) (dockerbuild.DockerGameOptions, error)
	}{
		{
			name: "server",
			call: func(cfg *config.Config) (dockerbuild.DockerGameOptions, error) {
				return ResolveContainerServerBuildOptions(cfg, "podman", "Shipping")
			},
		},
		{
			name: "client",
			call: func(cfg *config.Config) (dockerbuild.DockerGameOptions, error) {
				return ResolveContainerClientBuildOptions(cfg, "podman", "Linux")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := containerTestConfig()
			cfg.Engine.DockerImage = "my.repo/engine:5.7.3"

			opts, err := tt.call(cfg)
			if err != nil {
				t.Fatalf("resolve error = %v", err)
			}
			if opts.EngineImage != "my.repo/engine:5.7.3" {
				t.Errorf("EngineImage = %q, want my.repo/engine:5.7.3", opts.EngineImage)
			}
			if opts.Arch != "amd64" {
				t.Errorf("Arch = %q, want amd64", opts.Arch)
			}
		})
	}
}

func TestResolveContainerBuildOptionsInvalidDDC(t *testing.T) {
	origCfg, origMode := Cfg, DDCMode
	t.Cleanup(func() { Cfg, DDCMode = origCfg, origMode })
	Cfg = &config.Config{}
	DDCMode = "invalid"

	calls := map[string]func() (dockerbuild.DockerGameOptions, error){
		"server": func() (dockerbuild.DockerGameOptions, error) {
			return ResolveContainerServerBuildOptions(Cfg, "docker", "")
		},
		"client": func() (dockerbuild.DockerGameOptions, error) {
			return ResolveContainerClientBuildOptions(Cfg, "docker", "Linux")
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if _, err := call(); err == nil {
				t.Error("expected invalid DDC mode error")
			}
		})
	}
}
