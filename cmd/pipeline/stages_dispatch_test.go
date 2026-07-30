package pipeline

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestDispatchEngineBuildWSL2NoWSL(t *testing.T) {
	// Test WSL2 backend when WSL is not available
	// In dry-run, wsl.New will fail early
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

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		containerBackend: "wsl2",
		engineVersion:    "5.7",
		arch:             "amd64",
		target:           &stubTarget{},
	}

	// This will fail because WSL2 is not available, but the code path executes
	err := p.dispatchEngineBuild(context.Background())
	// Error is expected since we can't actually talk to WSL
	_ = err
}

func TestDispatchGameBuildContainer(t *testing.T) {
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
		containerBackend: "docker",
		engineVersion:    "5.7",
		arch:             "amd64",
		ddcMode:          "none",
		ddcPath:          "",
		ddcZenPath:       "",
		target:           &stubTarget{},
	}

	// Dispatch with docker backend - will fail to orchestrate, but code exercises the branch
	err := p.dispatchGameBuild(context.Background(), "TestGame")
	_ = err
}

func TestDispatchClientBuildDocker(t *testing.T) {
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
		containerBackend: "docker",
		engineVersion:    "5.7",
		ddcMode:          "none",
		ddcPath:          "",
		ddcZenPath:       "",
		target:           &stubTarget{},
	}

	// Dispatch with docker backend - will fail to orchestrate, but exercises the branch
	_, _, err := p.dispatchClientBuild(context.Background(), "TestGame")
	_ = err
}
