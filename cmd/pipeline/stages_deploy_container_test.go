package pipeline

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestStageContainerBuildCacheHit(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
		Container: config.ContainerConfig{
			ImageName: "test-image",
			Tag:       "latest",
		},
	}

	globals.SetGlobals(t, cfg)

	bc := newTestCache()
	containerHash := cache.ContainerKey(cfg, "C:/build/out")
	bc.Set(cache.StageContainerBuild, containerHash, "2024-01-01T00:00:00Z")

	p := &pipelineCtx{
		cfg:            cfg,
		r:              globals.NewRunner(),
		arch:           "amd64",
		serverBuildDir: "C:/build/out",
		buildCache:     bc,
		target:         &stubTarget{},
	}

	err := p.stageContainerBuild(context.Background())
	if err != nil {
		t.Fatalf("stageContainerBuild() error = %v, want nil", err)
	}
}

func TestStageContainerBuildDryRun(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
		Container: config.ContainerConfig{
			ImageName:  "test-image",
			Tag:        "latest",
			ServerPort: 7777,
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	// On a cross-arch build (target arch != runtime.GOARCH, e.g. amd64 on the
	// arm64 macOS runner) internal/container shells out to
	// `docker buildx inspect --bootstrap` before any dry-run guard and requires
	// the target platform in its output. The probe does not run on an amd64 host,
	// so stub docker with both platforms listed to keep the test host-independent
	// — the macOS runner has no docker on PATH at all.
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{
		Stdout: "Platforms: linux/amd64, linux/arm64",
	})

	r, getLines := testsupport.RecordingRunner()

	bc := newTestCache()

	p := &pipelineCtx{
		cfg:            cfg,
		r:              r,
		arch:           "amd64",
		serverBuildDir: filepath.Join(t.TempDir(), "out"),
		buildCache:     bc,
		target:         &stubTarget{},
	}

	if err := p.stageContainerBuild(context.Background()); err != nil {
		t.Fatalf("stageContainerBuild() error = %v, want nil", err)
	}

	lines := getLines()
	for _, want := range []string{"docker build", "--platform linux/amd64", "-t test-image:latest"} {
		if !findInLines(lines, want) {
			t.Errorf("recorded commands missing %q, got: %v", want, lines)
		}
	}
}

func TestStageContainerPushDryRun(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "my-game",
		},
		Container: config.ContainerConfig{
			ImageName: "test-image",
			Tag:       "v1.0",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return &stubTarget{}, nil
	})

	p := &pipelineCtx{
		cfg:    cfg,
		r:      r,
		arch:   "amd64",
		target: &stubTarget{},
	}

	err := p.stageContainerPush(context.Background())
	if err != nil {
		t.Fatalf("stageContainerPush() error = %v, want nil", err)
	}
}

func TestBuildImageURINoRepo(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "",
		},
		Container: config.ContainerConfig{
			Tag: "v1.0",
		},
	}

	globals.SetGlobals(t, cfg)

	p := newTestPipelineCtx(t, cfg, nil)

	_, err := p.buildImageURI(context.Background())
	// Should fail due to missing repository
	if err == nil {
		t.Errorf("buildImageURI() with missing repository expected error, got nil")
	}
}

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

	p := newTestPipelineCtx(t, cfg, nil)

	_, err := p.buildImageURI(context.Background())
	// Should fail due to missing tag
	if err == nil {
		t.Errorf("buildImageURI() with missing tag expected error, got nil")
	}
}

func TestBaseDockerGameOptsSuccess(t *testing.T) {
	engineRoot, projectPath, cfg := setupTestContext(t, "TestGame")

	cfg.Engine.DockerImageName = "my-engine"
	cfg.Container.Tag = "v1.0"

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	p := newTestPipelineCtx(t, cfg, &testContextOpts{
		containerBackend: "docker",
		fullVersion:      "5.7.3",
		ddcMode:          "zen",
		ddcPath:          "C:/ludus/ddc",
		ddcZenPath:       "/home/ue/.config/Epic/UnrealEngine/Common/Zen/Data",
	})

	opts, err := p.baseDockerGameOpts()
	if err != nil {
		t.Fatalf("baseDockerGameOpts() error = %v, want nil", err)
	}
	if opts.EngineImage == "" {
		t.Errorf("baseDockerGameOpts() EngineImage is empty")
	}

	_ = engineRoot
	_ = projectPath
}
