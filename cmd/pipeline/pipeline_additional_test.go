package pipeline

import (
	"context"
	"fmt"
	"strings"
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
	// Verify printNextStep with default flags (no session, no deploy skip)
	orig := skipDeploy
	origSession := withSession
	t.Cleanup(func() {
		skipDeploy = orig
		withSession = origSession
	})

	skipDeploy = false
	withSession = false

	output := captureStdout(printNextStep)
	if !strings.Contains(output, "Pipeline complete") {
		t.Errorf("expected 'Pipeline complete' in output, got: %q", output)
	}
	if !strings.Contains(output, "ludus deploy session") {
		t.Errorf("expected 'ludus deploy session' guidance, got: %q", output)
	}
	if strings.Contains(output, "ludus connect") {
		t.Errorf("should not show 'ludus connect' when withSession is false, got: %q", output)
	}
}

func TestPrintNextStepWithConnection(t *testing.T) {
	// Verify printNextStep with session enabled shows connect command
	orig := skipDeploy
	origSession := withSession
	t.Cleanup(func() {
		skipDeploy = orig
		withSession = origSession
	})

	skipDeploy = false
	withSession = true

	output := captureStdout(printNextStep)
	if !strings.Contains(output, "Pipeline complete") {
		t.Errorf("expected 'Pipeline complete' in output, got: %q", output)
	}
	if !strings.Contains(output, "ludus connect") {
		t.Errorf("expected 'ludus connect' guidance, got: %q", output)
	}
	if strings.Contains(output, "ludus deploy session") {
		t.Errorf("should not show 'ludus deploy session' when withSession is true, got: %q", output)
	}
}

func TestStageValidatePrebuiltImage(t *testing.T) {
	// Test validate with prebuilt image (skips engine source checks for docker backend)
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "Lyra")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  engineRoot,
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "Lyra",
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
	}

	globals.SetGlobals(t, cfg)

	p := &pipelineCtx{
		cfg:              cfg,
		containerBackend: "docker",
		target:           &stubTarget{},
	}

	err := p.stageValidate(context.Background())
	// With a prebuilt image on docker backend, engine source checks are skipped
	if err != nil {
		t.Fatalf("stageValidate() with prebuilt image expected no error, got: %v", err)
	}
}
