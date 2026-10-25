package pipeline

import (
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

// pipelineOptsCfg returns a config whose resolved fields (targets, arch,
// skip-cook, archive roots) are deterministic for option-assembly tests.
func pipelineOptsCfg() *config.Config {
	return &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "/engine",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
		},
		Game: config.GameConfig{
			ProjectName:  "TestGame",
			ProjectPath:  "/game/TestGame.uproject",
			ServerTarget: "TestGameServer",
			ClientTarget: "TestGameGame",
			Platform:     "Linux",
			Arch:         "arm64",
		},
	}
}

// TestPipelineGameOptsPropagateSkipCook pins #731: the pipeline hashes
// game.skipCook into its cache keys, so every backend it assembles must honor
// the flag — otherwise a run cooks content and records success under a
// skip-cook cache key.
func TestPipelineGameOptsPropagateSkipCook(t *testing.T) {
	cfg := pipelineOptsCfg()
	cfg.Game.SkipCook = true
	globals.SetGlobals(t, cfg)
	p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "podman"})

	tests := []struct {
		name string
		got  bool
	}{
		{name: "native server", got: p.nativeServerOpts("TestGame").SkipCook},
		{name: "native client", got: p.nativeClientOpts("TestGame").SkipCook},
	}
	for _, tt := range tests {
		if !tt.got {
			t.Errorf("%s: SkipCook = false, want true", tt.name)
		}
	}

	serverOpts, err := p.containerServerOpts()
	if err != nil {
		t.Fatalf("containerServerOpts() error = %v", err)
	}
	if !serverOpts.SkipCook {
		t.Error("container server: SkipCook = false, want true")
	}

	clientOpts, err := p.containerClientOpts()
	if err != nil {
		t.Fatalf("containerClientOpts() error = %v", err)
	}
	if !clientOpts.SkipCook {
		t.Error("container client: SkipCook = false, want true")
	}
}

// TestPipelineContainerServerOptsFields pins #698 and #750 for the pipeline's
// container server assembly: arch comes from config and the archive stays at
// the PackagedServer root.
func TestPipelineContainerServerOptsFields(t *testing.T) {
	cfg := pipelineOptsCfg()
	globals.SetGlobals(t, cfg)
	p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "podman"})

	opts, err := p.containerServerOpts()
	if err != nil {
		t.Fatalf("containerServerOpts() error = %v", err)
	}
	fields := map[string]struct{ got, want string }{
		"Arch":         {opts.Arch, "arm64"},
		"OutputDir":    {opts.OutputDir, config.ResolveServerArchiveRoot(cfg)},
		"ServerTarget": {opts.ServerTarget, "TestGameServer"},
	}
	for name, f := range fields {
		if f.got != f.want {
			t.Errorf("containerServerOpts().%s = %q, want %q", name, f.got, f.want)
		}
	}
}

// TestPipelineContainerClientOptsFields pins #750 for the pipeline's container
// client assembly: the archive root is PackagedClient, never the server root.
func TestPipelineContainerClientOptsFields(t *testing.T) {
	cfg := pipelineOptsCfg()
	globals.SetGlobals(t, cfg)
	p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "podman"})

	opts, err := p.containerClientOpts()
	if err != nil {
		t.Fatalf("containerClientOpts() error = %v", err)
	}
	fields := map[string]struct{ got, want string }{
		"OutputDir":      {opts.OutputDir, config.ResolveClientArchiveRoot(cfg)},
		"ClientPlatform": {opts.ClientPlatform, "Linux"},
		"ClientTarget":   {opts.ClientTarget, "TestGameGame"},
	}
	for name, f := range fields {
		if f.got != f.want {
			t.Errorf("containerClientOpts().%s = %q, want %q", name, f.got, f.want)
		}
	}
	if opts.OutputDir == config.ResolveServerArchiveRoot(cfg) {
		t.Errorf("client OutputDir %q must not be the server archive root", opts.OutputDir)
	}
}

// TestPipelineNativeOptsPropagateConfig covers the native assemblies' resolved
// fields beyond SkipCook: targets, arch, and platform.
func TestPipelineNativeOptsPropagateConfig(t *testing.T) {
	cfg := pipelineOptsCfg()
	globals.SetGlobals(t, cfg)
	p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "native", arch: "arm64"})

	server := p.nativeServerOpts("TestGame")
	fields := map[string]struct{ got, want string }{
		"ServerTarget": {server.ServerTarget, "TestGameServer"},
		"Arch":         {server.Arch, "arm64"},
		"Platform":     {server.Platform, "Linux"},
	}
	for name, f := range fields {
		if f.got != f.want {
			t.Errorf("nativeServerOpts().%s = %q, want %q", name, f.got, f.want)
		}
	}

	client := p.nativeClientOpts("TestGame")
	if client.ClientTarget != "TestGameGame" {
		t.Errorf("nativeClientOpts().ClientTarget = %q, want TestGameGame", client.ClientTarget)
	}
}
