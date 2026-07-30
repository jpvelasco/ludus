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

	r, _ := testsupport.RecordingRunner()

	bc := newTestCache()

	p := &pipelineCtx{
		cfg:            cfg,
		r:              r,
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
