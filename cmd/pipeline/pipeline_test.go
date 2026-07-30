package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestRunPipelineSuccess(t *testing.T) {
	// Test runPipeline with successful execution path
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
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
	backend = "native"
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
