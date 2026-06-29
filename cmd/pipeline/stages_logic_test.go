package pipeline

import (
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestResolveServerBuildDir(t *testing.T) {
	t.Run("explicit project path", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Game.ProjectPath = filepath.Join("games", "MyGame", "MyGame.uproject")
		got := resolveServerBuildDir(cfg, "amd64")
		want := filepath.Join("games", "MyGame", "PackagedServer", config.ServerPlatformDir("amd64"))
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("lyra default from engine source", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Game.ProjectName = "Lyra"
		cfg.Engine.SourcePath = filepath.Join("opt", "UnrealEngine")
		got := resolveServerBuildDir(cfg, "arm64")
		want := filepath.Join("opt", "UnrealEngine", "Samples", "Games", "Lyra",
			"PackagedServer", config.ServerPlatformDir("arm64"))
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestEngineImageName(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		p := &pipelineCtx{cfg: &config.Config{}}
		if got := p.engineImageName(); got != "ludus-engine" {
			t.Errorf("got %q, want ludus-engine", got)
		}
	})
	t.Run("configured", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Engine.DockerImageName = "custom-engine"
		p := &pipelineCtx{cfg: cfg}
		if got := p.engineImageName(); got != "custom-engine" {
			t.Errorf("got %q, want custom-engine", got)
		}
	})
}

func TestUsesPrebuiltImage(t *testing.T) {
	// Prebuilt image is honored only with a container backend; native/wsl2 build
	// from source even when engine.dockerImage is set (regression guard for #394
	// review feedback).
	tests := []struct {
		name        string
		dockerImage string
		backend     string
		want        bool
	}{
		{"docker image + docker backend", "ecr/img:tag", "docker", true},
		{"docker image + podman backend", "ecr/img:tag", "podman", true},
		{"docker image + native backend", "ecr/img:tag", "native", false},
		{"docker image + wsl2 backend", "ecr/img:tag", "wsl2", false},
		{"no image + docker backend", "", "docker", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Engine.DockerImage = tt.dockerImage
			p := &pipelineCtx{cfg: cfg, containerBackend: tt.backend}
			if got := p.usesPrebuiltImage(); got != tt.want {
				t.Errorf("usesPrebuiltImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newTestCache() *cache.Cache {
	return &cache.Cache{Entries: make(map[cache.StageKey]*cache.Entry)}
}

func TestCheckCacheSkip(t *testing.T) {
	const stage = cache.StageKey("engine")

	t.Run("noCache flag disables skip", func(t *testing.T) {
		orig := noCache
		t.Cleanup(func() { noCache = orig })
		noCache = true
		p := &pipelineCtx{buildCache: newTestCache()}
		if p.checkCacheSkip(stage, "hash1", "Engine") {
			t.Error("expected no skip when noCache is set")
		}
	})

	t.Run("hit returns true", func(t *testing.T) {
		orig := noCache
		t.Cleanup(func() { noCache = orig })
		noCache = false
		c := newTestCache()
		c.Set(stage, "hash1", "2026-01-01T00:00:00Z")
		p := &pipelineCtx{buildCache: c}
		if !p.checkCacheSkip(stage, "hash1", "Engine") {
			t.Error("expected skip on cache hit")
		}
	})

	t.Run("miss returns false", func(t *testing.T) {
		orig := noCache
		t.Cleanup(func() { noCache = orig })
		noCache = false
		c := newTestCache()
		c.Set(stage, "oldhash", "2026-01-01T00:00:00Z")
		p := &pipelineCtx{buildCache: c}
		if p.checkCacheSkip(stage, "newhash", "Engine") {
			t.Error("expected no skip when hash differs")
		}
	})
}
