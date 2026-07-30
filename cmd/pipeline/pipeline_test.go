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

func TestRunPipelineSuccess(t *testing.T) {
	// Test runPipeline with successful execution path.
	// Uses a prebuilt image to skip environment-dependent checks (disk/memory).
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  engineRoot,
			DockerImage: "my.repo/engine:5.7.3", // Prebuilt image relaxes prerequisite checks
			Version:     "5.7.3",
			MaxJobs:     1,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "test-game",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	// Swap in our stub target
	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return &stubTarget{name: "binary", caps: deploy.Capabilities{}}, nil
	})

	// Set up flags
	origSkipEngine := skipEngine
	origSkipGame := skipGame
	origSkipContainer := skipContainer
	origSkipDeploy := skipDeploy
	origWithClient := withClient
	origWithSession := withSession
	origBackend := backend
	origNoCache := noCache
	t.Cleanup(func() {
		skipEngine = origSkipEngine
		skipGame = origSkipGame
		skipContainer = origSkipContainer
		skipDeploy = origSkipDeploy
		withClient = origWithClient
		withSession = origWithSession
		backend = origBackend
		noCache = origNoCache
	})

	// Use skip flags to speed up the test
	skipEngine = true
	skipGame = true
	skipContainer = true
	skipDeploy = true
	withClient = false
	withSession = false
	backend = "docker"
	noCache = false

	cmd := Cmd
	cmd.SetContext(context.Background())

	err := runPipeline(cmd, nil)
	if err != nil {
		t.Fatalf("runPipeline() error = %v, want nil", err)
	}
}

func TestRunPipelineValidationError(t *testing.T) {
	// Test runPipeline error handling when validation fails
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "/nonexistent/engine",
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return &stubTarget{name: "binary", caps: deploy.Capabilities{}}, nil
	})

	// Set up flags
	origSkipEngine := skipEngine
	origSkipGame := skipGame
	origSkipContainer := skipContainer
	origSkipDeploy := skipDeploy
	origWithClient := withClient
	origWithSession := withSession
	origBackend := backend
	origNoCache := noCache
	t.Cleanup(func() {
		skipEngine = origSkipEngine
		skipGame = origSkipGame
		skipContainer = origSkipContainer
		skipDeploy = origSkipDeploy
		withClient = origWithClient
		withSession = origWithSession
		backend = origBackend
		noCache = origNoCache
	})

	skipEngine = true
	skipGame = true
	skipContainer = true
	skipDeploy = true
	withClient = false
	withSession = false
	backend = "native"
	noCache = false

	cmd := Cmd
	cmd.SetContext(context.Background())

	// Validation will fail due to nonexistent engine source
	err := runPipeline(cmd, nil)
	if err == nil {
		t.Fatal("runPipeline() expected error from validation, got nil")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected error message to contain 'failed', got: %v", err)
	}
}

func TestNewPipelineCtxSuccess(t *testing.T) {
	engineRoot, projectPath, cfg := setupTestContext(t, "TestGame")

	cfg.AWS = config.AWSConfig{
		AccountID:     "123456789012",
		Region:        "us-east-1",
		ECRRepository: "test-game",
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

	_ = engineRoot
	_ = projectPath
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

func TestPrintNextStepWithDeploySkipped(t *testing.T) {
	// Verify printNextStep when deploy is skipped shows simpler guidance
	origSkip := skipDeploy
	origSession := withSession
	t.Cleanup(func() {
		skipDeploy = origSkip
		withSession = origSession
	})

	skipDeploy = true
	withSession = false

	output := captureStdout(printNextStep)
	if !strings.Contains(output, "Pipeline complete") {
		t.Errorf("expected 'Pipeline complete', got: %q", output)
	}
	// When deploy is skipped, should not show deploy guidance
	if strings.Contains(output, "ludus deploy session") || strings.Contains(output, "ludus connect") {
		t.Errorf("should not show deploy/connect guidance when skipDeploy is true, got: %q", output)
	}
}
