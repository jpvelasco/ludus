package pipeline

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestStageEngineBuildCacheMissRecord(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t)

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	bc := newTestCache()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                globals.NewRunner(),
		engineHash:       "some_hash",
		buildCache:       bc,
		target:           &stubTarget{},
		containerBackend: "native",
		engineVersion:    "5.7",
		arch:             "amd64",
	}

	// Perform the stage - should execute (cache miss) and record cache
	err := p.stageEngineBuild(context.Background())
	if err != nil {
		t.Fatalf("stageEngineBuild() error = %v, want nil", err)
	}

	// Verify cache was recorded (but skipped in dry-run)
	// In dry-run, cache is not persisted
}

func TestMissReasonOutput(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "C:/ue5",
		},
	}

	bc := newTestCache()
	bc.Set(cache.StageEngine, "old_hash", "2024-01-01T00:00:00Z")

	p := &pipelineCtx{
		cfg:        cfg,
		buildCache: bc,
	}

	got := p.checkCacheSkip(cache.StageEngine, "new_hash", "Engine")
	if got {
		t.Errorf("checkCacheSkip() for different hash = true, want false")
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

	p := &pipelineCtx{
		cfg: cfg,
	}

	_, err := p.buildImageURI(context.Background())
	// Should fail due to missing repository
	if err == nil {
		t.Errorf("buildImageURI() with missing repository expected error, got nil")
	}
}

func TestPrintNextStepWithDeploy(t *testing.T) {
	// Verify printNextStep with default flags (shows deploy guidance)
	skipDeploy = false
	withSession = false

	t.Cleanup(func() {
		skipDeploy = false
	})

	// Just verify it doesn't panic
	printNextStep()
}

func TestPrintNextStepSkipDeploy(t *testing.T) {
	// Verify printNextStep when deploy is skipped
	skipDeploy = true

	t.Cleanup(func() {
		skipDeploy = false
	})

	// Just verify it doesn't panic
	printNextStep()
}
