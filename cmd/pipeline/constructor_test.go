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

// TestNewPipelineCtxBinaryTargetNoAWS pins the issue-693 contract at the
// pipeline entry point: a binary-export pipeline resolves entirely locally —
// no AWS account, region, or ECR configuration may be required before the
// stages even run.
func TestNewPipelineCtxBinaryTargetNoAWS(t *testing.T) {
	t.Chdir(t.TempDir())
	_, _, cfg := setupTestContext(t, "TestGame")
	cfg.Deploy = config.DeployConfig{
		Target:    "binary",
		OutputDir: filepath.Join(t.TempDir(), "out"),
	}
	// AWS fields stay zero on purpose.

	globals.SetGlobals(t, cfg)

	p, err := newPipelineCtx(&cobra.Command{Use: "run"})
	if err != nil {
		t.Fatalf("newPipelineCtx() error = %v, want nil (binary target needs no AWS)", err)
	}
	defer globals.CloseBuildLog()

	if p.target == nil || p.target.Name() != "binary" {
		t.Fatalf("target = %v, want binary", p.target)
	}
	caps := p.target.Capabilities()
	if caps.NeedsAWS || caps.NeedsImageURI {
		t.Errorf("binary capabilities = %+v, want no AWS and no image URI", caps)
	}
}

// TestNewPipelineCtxEngineKeyUsesEffectiveBackend pins the #557 contract: the
// engine cache key must hash the EFFECTIVE backend (CLI --backend wins over
// ludus.yaml), not the raw YAML value captured before flag resolution.
func TestNewPipelineCtxEngineKeyUsesEffectiveBackend(t *testing.T) {
	t.Chdir(t.TempDir())
	backend = "docker"
	t.Cleanup(func() { backend = "" })

	cfg := &config.Config{
		Deploy: config.DeployConfig{Target: "binary"},
		Engine: config.EngineConfig{SourcePath: filepath.Join(t.TempDir(), "ue5"), Version: "5.7.3"},
	}
	globals.SetGlobals(t, cfg)

	p, err := newPipelineCtx(&cobra.Command{Use: "run"})
	if err != nil {
		t.Fatalf("newPipelineCtx() error = %v", err)
	}
	defer globals.CloseBuildLog()

	expected := &config.Config{}
	*expected = *cfg
	expected.Engine.Backend = "docker"
	if p.engineHash != cache.EngineKey(expected) {
		t.Error("engineHash does not reflect the effective --backend; switching backends would reuse foreign cache entries")
	}
	if p.containerBackend != "docker" {
		t.Errorf("containerBackend = %q, want docker", p.containerBackend)
	}
}
