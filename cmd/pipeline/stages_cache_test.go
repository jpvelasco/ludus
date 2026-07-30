package pipeline

import (
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestRecordCache(t *testing.T) {
	original := globals.DryRun
	t.Cleanup(func() { globals.DryRun = original })
	t.Chdir(t.TempDir())
	const stage = cache.StageKey("game-server")

	t.Run("records normal build", func(t *testing.T) {
		globals.DryRun = false
		c := newTestCache()
		(&pipelineCtx{buildCache: c}).recordCache(stage, "hash1")
		if !c.IsHit(stage, "hash1") {
			t.Error("cache entry was not recorded")
		}
	})

	t.Run("ignores dry run", func(t *testing.T) {
		globals.DryRun = true
		c := newTestCache()
		(&pipelineCtx{buildCache: c}).recordCache(stage, "hash1")
		if c.IsHit(stage, "hash1") {
			t.Error("dry-run cache entry was recorded")
		}
	})
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
