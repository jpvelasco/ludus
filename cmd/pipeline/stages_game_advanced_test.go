package pipeline

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestDispatchClientBuildNative(t *testing.T) {
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
		cfg:              cfg,
		r:                r,
		containerBackend: "native",
		engineVersion:    "5.7",
		ddcMode:          "none",
		ddcPath:          "",
		target:           &stubTarget{},
	}

	result, label, err := p.dispatchClientBuild(context.Background(), "TestGame")
	if err != nil {
		t.Fatalf("dispatchClientBuild() error = %v, want nil", err)
	}
	if label != "" {
		t.Errorf("dispatchClientBuild() label = %q, want empty for native", label)
	}
	if result == nil {
		t.Errorf("dispatchClientBuild() result = nil, want non-nil")
	}
}

func TestStageGameBuildNative(t *testing.T) {
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

	bc := newTestCache()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		containerBackend: "native",
		engineVersion:    "5.7",
		arch:             "amd64",
		ddcMode:          "none",
		ddcPath:          "",
		serverHash:       "test_hash",
		buildCache:       bc,
		target:           &stubTarget{},
	}

	err := p.stageGameBuild(context.Background())
	if err != nil {
		t.Fatalf("stageGameBuild() error = %v, want nil", err)
	}
}

func TestStageClientBuildNative(t *testing.T) {
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

	bc := newTestCache()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		containerBackend: "native",
		engineVersion:    "5.7",
		ddcMode:          "none",
		ddcPath:          "",
		clientHash:       "test_hash",
		buildCache:       bc,
		target:           &stubTarget{},
	}

	err := p.stageClientBuild(context.Background())
	if err != nil {
		t.Fatalf("stageClientBuild() error = %v, want nil", err)
	}
}
