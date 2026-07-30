package pipeline

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestBuildEngineContainerDryRun(t *testing.T) {
	// Test buildEngineContainer with dry-run: verifies command orchestration.
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

	r, getLines := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		containerBackend: "docker",
		fullVersion:      "5.7.3",
		engineVersion:    "5.7",
		arch:             "amd64",
		target:           &stubTarget{},
	}

	// In dry-run, buildEngineContainer orchestrates without spawning processes
	err := p.buildEngineContainer(context.Background())
	if err != nil {
		t.Fatalf("buildEngineContainer() error = %v, want nil", err)
	}

	// Verify that docker build command was recorded (not actually executed)
	lines := getLines()
	if len(lines) == 0 {
		t.Errorf("expected recorded command lines for docker build, got none")
	}
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
