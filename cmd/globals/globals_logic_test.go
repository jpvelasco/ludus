package globals

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

func TestResolveBackend(t *testing.T) {
	origCfg := Cfg
	t.Cleanup(func() { Cfg = origCfg })

	t.Run("flag takes precedence over config", func(t *testing.T) {
		Cfg = &config.Config{}
		Cfg.Engine.Backend = "native"
		if got := ResolveBackend("wsl2"); got != "wsl2" {
			t.Errorf("got %q, want flag value wsl2", got)
		}
	})

	t.Run("falls back to config when flag empty", func(t *testing.T) {
		Cfg = &config.Config{}
		Cfg.Engine.Backend = "docker"
		if got := ResolveBackend(""); got != "docker" {
			t.Errorf("got %q, want config value docker", got)
		}
	})

	t.Run("empty when no flag and no config", func(t *testing.T) {
		Cfg = nil
		if got := ResolveBackend(""); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestResolveEngineImageParts(t *testing.T) {
	// With Engine.DockerImage set, ResolveEngineImage short-circuits before any
	// state/version detection, so this is deterministic.
	t.Run("tagged image splits into name and tag", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Engine.DockerImage = "my-registry/engine:5.7"
		name, tag, err := ResolveEngineImageParts(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "my-registry/engine" || tag != "5.7" {
			t.Errorf("got (%q, %q), want (my-registry/engine, 5.7)", name, tag)
		}
	})

	t.Run("untagged image defaults to latest", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Engine.DockerImage = "ludus-engine"
		name, tag, err := ResolveEngineImageParts(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "ludus-engine" || tag != "latest" {
			t.Errorf("got (%q, %q), want (ludus-engine, latest)", name, tag)
		}
	})

	t.Run("digest reference errors", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Engine.DockerImage = "my-registry/engine@sha256:abc"
		if _, _, err := ResolveEngineImageParts(cfg); err == nil {
			t.Error("expected error for digest reference")
		}
	})
}

func TestWarnIfLegacyDDC(t *testing.T) {
	origMode := DDCMode
	origCfg := Cfg
	t.Cleanup(func() {
		DDCMode = origMode
		Cfg = origCfg
	})

	// Smoke: exercises both the legacy (warn) and non-legacy (silent) branches
	// without asserting on stderr. Must not panic and must not error internally.
	Cfg = &config.Config{}

	DDCMode = "local"
	WarnIfLegacyDDC() // legacy → prints to stderr

	DDCMode = "zen"
	WarnIfLegacyDDC() // non-legacy → silent

	DDCMode = "invalid-mode"
	WarnIfLegacyDDC() // invalid → ignored (no panic)
}

func TestBaseDockerGameOptions(t *testing.T) {
	cfg := &config.Config{}
	cfg.Game.ProjectPath = "/games/MyGame/MyGame.uproject"
	cfg.Game.ProjectName = "MyGame"

	opts := BaseDockerGameOptions(cfg, "ludus-engine:5.7", "5.7", "zen", "/ddc", "/zen", "docker")

	fields := map[string]struct{ got, want string }{
		"EngineImage":   {opts.EngineImage, "ludus-engine:5.7"},
		"ProjectPath":   {opts.ProjectPath, cfg.Game.ProjectPath},
		"ProjectName":   {opts.ProjectName, "MyGame"},
		"EngineVersion": {opts.EngineVersion, "5.7"},
		"DDCMode":       {opts.DDCMode, "zen"},
		"DDCPath":       {opts.DDCPath, "/ddc"},
		"DDCZenPath":    {opts.DDCZenPath, "/zen"},
		"Runtime":       {opts.Runtime, "docker"},
	}
	for name, f := range fields {
		if f.got != f.want {
			t.Errorf("%s = %q, want %q", name, f.got, f.want)
		}
	}
	if opts.OutputDir == "" {
		t.Error("OutputDir should be derived from project path, got empty")
	}
}

func TestResolveTarget_Binary(t *testing.T) {
	// The binary target is the only one that doesn't touch AWS, so it's
	// deterministically testable. Default output dir applies when unset.
	cfg := &config.Config{}
	cfg.Deploy.Target = "binary"
	target, err := ResolveTarget(context.Background(), cfg, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target == nil {
		t.Fatal("expected a non-nil binary target")
	}
}

func TestResolveTarget_BinaryOverride(t *testing.T) {
	// targetOverride takes precedence over cfg.Deploy.Target.
	cfg := &config.Config{}
	cfg.Deploy.Target = "gamelift" // would hit AWS; override to binary
	target, err := ResolveTarget(context.Background(), cfg, "binary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target == nil {
		t.Fatal("expected a non-nil binary target")
	}
}

func TestResolveTarget_Unknown(t *testing.T) {
	cfg := &config.Config{}
	_, err := ResolveTarget(context.Background(), cfg, "nonsense")
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
}
