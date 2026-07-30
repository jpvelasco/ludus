package pipeline

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestStageEngineBuildSkipsOnCacheHit(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t)
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			MaxJobs:    1,
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg)

	// Pre-populate cache to force skip
	bc := newTestCache()
	engineKey := cache.EngineKey(cfg)
	bc.Set(cache.StageEngine, engineKey, "2024-01-01T00:00:00Z")

	p := &pipelineCtx{
		cfg:        cfg,
		r:          globals.NewRunner(),
		engineHash: engineKey,
		buildCache: bc,
		target:     &stubTarget{name: "binary", caps: deploy.Capabilities{}},
	}

	err := p.stageEngineBuild(context.Background())
	if err != nil {
		t.Fatalf("stageEngineBuild() error = %v, want nil", err)
	}
}

func TestDispatchEngineBuildNative(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			MaxJobs:    1,
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, getLines := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		containerBackend: "native",
		engineVersion:    "5.7",
		engineHash:       "test_hash",
		target:           &stubTarget{name: "binary", caps: deploy.Capabilities{}},
	}

	err := p.dispatchEngineBuild(context.Background())
	if err != nil {
		t.Fatalf("dispatchEngineBuild() error = %v, want nil", err)
	}

	lines := getLines()
	hasShaderCompile := findInLines(lines, "ShaderCompileWorker")
	if !hasShaderCompile {
		t.Errorf("expected ShaderCompileWorker in recorded lines, got: %v", lines)
	}
}

func TestDispatchEngineBuildContainerPrebuiltImage(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			DockerImage: "my.repo/engine:5.7.3",
			Version:     "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		containerBackend: "docker",
		engineVersion:    "5.7",
		fullVersion:      "5.7.3",
		target:           &stubTarget{name: "gamelift", caps: deploy.Capabilities{NeedsContainerBuild: true}},
	}

	err := p.dispatchEngineBuild(context.Background())
	if err != nil {
		t.Fatalf("dispatchEngineBuild() error = %v, want nil", err)
	}
}

func TestBuildEngineContainerPrebuiltImage(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			DockerImage: "my.repo/engine:5.7.3",
		},
	}

	globals.SetGlobals(t, cfg)

	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		containerBackend: "docker",
		fullVersion:      "5.7.3",
		target:           &stubTarget{},
	}

	err := p.buildEngineContainer(context.Background())
	if err != nil {
		t.Fatalf("buildEngineContainer() error = %v, want nil", err)
	}
}
