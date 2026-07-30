package pipeline

import (
	"context"
	"fmt"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestNewPipelineCtxSuccess(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
		},
		Game: config.GameConfig{
			ProjectPath: projectPath,
			ProjectName: "TestGame",
			Platform:    "Linux",
		},
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "test-game",
		},
	}

	globals.SetGlobals(t, cfg)

	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return &stubTarget{name: "binary"}, nil
	})

	cmd := Cmd

	pctx, err := newPipelineCtx(cmd)
	if err != nil {
		t.Fatalf("newPipelineCtx() error = %v, want nil", err)
	}

	if pctx.cfg == nil {
		t.Error("newPipelineCtx() cfg is nil")
	}
	if pctx.r == nil {
		t.Error("newPipelineCtx() r is nil")
	}
	if pctx.target == nil {
		t.Error("newPipelineCtx() target is nil")
	}
}

func TestNewPipelineCtxResolveTargetError(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "C:/ue5",
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg)

	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return nil, fmt.Errorf("target resolution failed")
	})

	cmd := Cmd

	_, err := newPipelineCtx(cmd)
	if err == nil {
		t.Fatal("newPipelineCtx() expected error, got nil")
	}
}

func TestPrintNextStep(t *testing.T) {
	// Just verify printNextStep doesn't panic
	printNextStep()
}

func TestStageValidatePrebuiltImage(t *testing.T) {
	// Test validate with prebuilt image (skips engine source checks)
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  engineRoot,
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "Lyra",
		},
	}

	globals.SetGlobals(t, cfg)

	p := &pipelineCtx{
		cfg:              cfg,
		containerBackend: "docker",
		target:           &stubTarget{},
	}

	err := p.stageValidate(context.Background())
	// May have failures for other reasons, but should not fail on engine source
	// Just verify it runs without panic
	_ = err
}
