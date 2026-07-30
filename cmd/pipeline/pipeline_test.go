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

// TestRunPipelineSuccess drives runPipeline end to end with every stage skipped,
// asserting the run announces the dry run, walks the stage list, and completes.
//
// The Validate stage runs regardless of the skip flags and includes Disk Space and
// Memory checks against real host hardware (300 GB free / 16 GB RAM), which CI
// runners do not have and no fixture can fake. So this asserts the orchestration
// output rather than `err == nil`: a machine-dependent pass/fail assertion here
// would be green on a dev box and red on CI, which is exactly what happened
// before. stageValidate's own behavior is covered by TestStageValidatePrebuiltImage.
func TestRunPipelineSuccess(t *testing.T) {
	stubCrossCompileToolchain(t)
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

	// prereq shells out to aws/docker; stub them so validation runs offline
	// rather than waiting on a real `aws sts get-caller-identity`.
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"aws":    {Stdout: `{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/test"}`},
		"docker": {},
	})

	cmd := Cmd
	cmd.SetContext(context.Background())

	var err error
	output := captureStdout(func() { err = runPipeline(cmd, nil) })

	// Only the host-hardware checks may fail here; anything else is a real defect.
	if err != nil && !strings.Contains(err.Error(), "Validate prerequisites") {
		t.Fatalf("runPipeline() error = %v, want nil or a Validate-stage failure", err)
	}

	// Every stage is skipped by the flags above, so the run announces the dry run
	// and walks the stage list. It only reaches "Pipeline complete." when Validate
	// passed, which depends on host disk and RAM — so that line is asserted only
	// when no error came back.
	for _, want := range []string{"Dry run", "Validate prerequisites"} {
		if !strings.Contains(output, want) {
			t.Errorf("runPipeline() output missing %q, got:\n%s", want, output)
		}
	}
	if err == nil && !strings.Contains(output, "Pipeline complete.") {
		t.Errorf("runPipeline() succeeded but output missing %q, got:\n%s", "Pipeline complete.", output)
	}
}

func TestRunPipelineValidationError(t *testing.T) {
	// Test runPipeline error handling when validation fails
	stubCrossCompileToolchain(t)

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

	// prereq shells out to aws/docker; stub them so validation runs offline
	// rather than waiting on a real `aws sts get-caller-identity`.
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"aws":    {Stdout: `{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/test"}`},
		"docker": {},
	})

	cmd := Cmd
	cmd.SetContext(context.Background())

	// Validation fails because the engine source path does not exist, and the
	// error must name the stage so the user knows where the run stopped.
	err := runPipeline(cmd, nil)
	if err == nil {
		t.Fatal("runPipeline() error = nil, want a validation failure")
	}
	for _, want := range []string{"Validate prerequisites", "prerequisite check"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("runPipeline() error = %v, want it to mention %q", err, want)
		}
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
