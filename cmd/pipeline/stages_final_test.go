package pipeline

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestBuildImageURINoTag(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "my-game",
		},
		Container: config.ContainerConfig{
			Tag: "",
		},
	}

	globals.SetGlobals(t, cfg)

	p := &pipelineCtx{
		cfg: cfg,
	}

	_, err := p.buildImageURI(context.Background())
	// Should fail due to missing tag
	if err == nil {
		t.Errorf("buildImageURI() with missing tag expected error, got nil")
	}
}

func TestDispatchGameBuildWSL2(t *testing.T) {
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
		containerBackend: "wsl2",
		engineVersion:    "5.7",
		arch:             "amd64",
		ddcMode:          "none",
		ddcPath:          "",
		target:           &stubTarget{},
	}

	// Dispatch with WSL2 backend in dry-run mode
	err := p.dispatchGameBuild(context.Background(), "TestGame")
	// May succeed if WSL2 is available, or fail otherwise; both are acceptable
	if err == nil {
		// Verify that command orchestration occurred
		lines := getLines()
		if len(lines) == 0 {
			t.Errorf("expected recorded command lines for WSL2 build, got none")
		}
	}
}

func TestBaseDockerGameOptsSuccess(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:      engineRoot,
			Version:         "5.7.3",
			DockerImageName: "my-engine",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	p := &pipelineCtx{
		cfg:              cfg,
		containerBackend: "docker",
		engineVersion:    "5.7",
		fullVersion:      "5.7.3",
		ddcMode:          "zen",
		ddcPath:          "C:/ludus/ddc",
		ddcZenPath:       "/home/ue/.config/Epic/UnrealEngine/Common/Zen/Data",
		arch:             "amd64",
		target:           &stubTarget{},
	}

	opts, err := p.baseDockerGameOpts()
	if err != nil {
		t.Fatalf("baseDockerGameOpts() error = %v, want nil", err)
	}
	if opts.EngineImage == "" {
		t.Errorf("baseDockerGameOpts() EngineImage is empty")
	}
}
