package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/devrecon/ludus/internal/runner"
)

// pipelineCtx holds shared state for pipeline stage execution.
type pipelineCtx struct {
	cfg              *config.Config
	r                *runner.Runner
	engineVersion    string
	containerBackend string
	ddcMode          string
	ddcPath          string
	arch             string
	serverBuildDir   string
	target           deploy.Target
	engineHash       string
	serverHash       string
	clientHash       string
	buildCache       *cache.Cache
	wslNative        bool
	wslDistro        string
}

// resolveBackend returns the effective backend, preferring CLI flag over config.
func resolveBackend() string { return globals.ResolveBackend(backend) }

// checkCacheSkip returns true if the stage can be skipped due to a cache hit.
// Prints cache status messages as a side effect.
func (p *pipelineCtx) checkCacheSkip(stage cache.StageKey, hash, label string) bool {
	if noCache {
		return false
	}
	if p.buildCache.IsHit(stage, hash) {
		fmt.Printf("    %s is up to date (cached), skipping.\n", label)
		return true
	}
	if reason := p.buildCache.MissReason(stage, hash); reason != "" {
		fmt.Printf("    Cache: %s\n", reason)
	}
	return false
}

// recordCache saves a cache entry for the given stage and hash.
func (p *pipelineCtx) recordCache(stage cache.StageKey, hash string) {
	p.buildCache.Set(stage, hash, time.Now().UTC().Format(time.RFC3339))
	_ = cache.Save(p.buildCache)
}

func (p *pipelineCtx) stageValidate(ctx context.Context) error {
	checker := prereq.NewChecker(p.cfg.Engine.SourcePath, p.cfg.Engine.Version, true, &p.cfg.Game)
	checker.Backend = p.containerBackend
	results := checker.RunAll()
	failed := 0
	for _, res := range results {
		marker := "[OK]"
		if !res.Passed {
			marker = "[FAIL]"
			failed++
		}
		fmt.Printf("    %-6s %s\n", marker, res.Name)
	}
	if failed > 0 {
		return fmt.Errorf("%d prerequisite check(s) failed", failed)
	}
	return nil
}
