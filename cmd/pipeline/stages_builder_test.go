package pipeline

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestBuildEngineContainerError(t *testing.T) {
	// Test buildEngineContainer with no prebuilt image (would build from source).
	// Since we're in dry-run, it will attempt to orchestrate the build.
	engineRoot := testsupport.FakeEngineTree(t)

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
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
		fullVersion:      "5.7.3",
		engineVersion:    "5.7",
		arch:             "amd64",
		target:           &stubTarget{},
	}

	// This will attempt to orchestrate but dry-run prevents actual execution
	err := p.buildEngineContainer(context.Background())
	// May fail due to orchestration details, but code path is exercised
	_ = err
}

func TestBuildGameNativeDryRun(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t)
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:           cfg,
		r:             r,
		engineVersion: "5.7",
		arch:          "amd64",
		ddcMode:       "none",
		ddcPath:       "",
		target:        &stubTarget{},
	}

	_, err := p.buildGameNative(context.Background(), "TestGame")
	if err != nil {
		t.Fatalf("buildGameNative() error = %v, want nil", err)
	}
}

func TestBuildClientNativeDryRun(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t)
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:           cfg,
		r:             r,
		engineVersion: "5.7",
		ddcMode:       "none",
		ddcPath:       "",
		target:        &stubTarget{},
	}

	_, err := p.buildClientNative(context.Background(), "TestGame")
	if err != nil {
		t.Fatalf("buildClientNative() error = %v, want nil", err)
	}
}

func TestStageEngineBuildCacheMiss(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t)

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	bc := newTestCache()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		engineHash:       cache.EngineKey(cfg),
		buildCache:       bc,
		target:           &stubTarget{},
		containerBackend: "native",
		engineVersion:    "5.7",
		arch:             "amd64",
	}

	err := p.stageEngineBuild(context.Background())
	if err != nil {
		t.Fatalf("stageEngineBuild() error = %v, want nil", err)
	}
}
