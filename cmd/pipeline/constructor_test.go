package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
)

// TestNewPipelineCtxCorruptCacheDegradesToEmpty pins the recovery contract for
// a corrupt .ludus/cache.json: constructing the pipeline must never return a
// nil build cache (stage checks dereference it) and the returned cache must be
// usable, so stages simply rerun instead of panicking.
func TestNewPipelineCtxCorruptCacheDegradesToEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".ludus", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".ludus", "cache.json"), []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}

	globals.SetGlobals(t, &config.Config{
		Deploy: config.DeployConfig{Target: "binary"},
	})

	p, err := newPipelineCtx(&cobra.Command{Use: "run"})
	if err != nil {
		t.Fatalf("newPipelineCtx() error = %v", err)
	}
	defer globals.CloseBuildLog()
	if p.buildCache == nil {
		t.Fatal("newPipelineCtx() buildCache = nil; stage checks would panic on a corrupt cache")
	}
	p.buildCache.Set(cache.StageEngine, "hash", "reason")
}
