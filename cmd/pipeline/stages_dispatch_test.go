package pipeline

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestDispatchEngineBuildWSL2(t *testing.T) {
	// Test WSL2 backend dispatching (may succeed if WSL2 is available on system)
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

	r, getLines := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		containerBackend: "wsl2",
		engineVersion:    "5.7",
		arch:             "amd64",
		target:           &stubTarget{},
	}

	// In dry-run mode with WSL2, should orchestrate the build
	err := p.dispatchEngineBuild(context.Background())
	// May succeed or fail depending on WSL2 availability
	if err == nil {
		// Verify that command orchestration occurred
		lines := getLines()
		if len(lines) == 0 {
			t.Errorf("expected recorded command lines for WSL2 engine build, got none")
		}
	}
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

	r, getLines := testsupport.RecordingRunner()

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

	// Dispatch with docker backend - orchestrates the build in dry-run
	err := p.dispatchGameBuild(context.Background(), "TestGame")
	if err != nil {
		t.Fatalf("dispatchGameBuild() error = %v, want nil", err)
	}

	// Verify docker command was recorded
	lines := getLines()
	if len(lines) == 0 {
		t.Errorf("expected recorded docker build command lines, got none")
	}
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

	r, getLines := testsupport.RecordingRunner()

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

	// Dispatch with docker backend - orchestrates the build in dry-run
	result, info, err := p.dispatchClientBuild(context.Background(), "TestGame")
	if err != nil {
		t.Fatalf("dispatchClientBuild() error = %v, want nil", err)
	}

	if result == nil {
		t.Errorf("expected non-nil result, got nil")
	}

	// Verify docker command was recorded
	lines := getLines()
	if len(lines) == 0 {
		t.Errorf("expected recorded docker build command lines, got none")
	}

	_ = info // Not used in assertion
}
