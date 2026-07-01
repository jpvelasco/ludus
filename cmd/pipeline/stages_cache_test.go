package pipeline

import (
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
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
