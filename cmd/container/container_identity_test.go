package container

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
)

// seedContainerCache writes a cache entry for the container build stage under
// the given hash, in a fresh working directory.
func seedContainerCache(t *testing.T, hash string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join(dir, ".ludus"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(&cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
		cache.StageContainerBuild: {Hash: hash, BuiltAt: "2026-01-01T00:00:00Z"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ludus", "cache.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// containerKeyFor computes the container cache key the way an effective build
// request would: the requested image name and tag applied to the config.
func containerKeyFor(cfg *config.Config, serverBuildDir, imageName, tag string) string {
	hashCfg := cfg.Clone()
	hashCfg.Container.ImageName = imageName
	hashCfg.Container.Tag = tag
	return cache.ContainerKey(&hashCfg, serverBuildDir)
}

// TestRunBuildTagFlagEntersCacheKey pins #692's tag repro: a set --tag flag
// overrides container.tag and names the image being built, so the cache key
// must include it. A warm entry for one CLI tag must not satisfy another.
func TestRunBuildTagFlagEntersCacheKey(t *testing.T) {
	resetContainerFlags(t)
	cfg, serverBuildDir := containerBuildConfig(t)
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithNoLogs(true))
	stubBuildPrereqTools(t)

	// Warm the cache the way a build without --tag does, under the config
	// tag — the effective tag the cache key must hash.
	seedContainerCache(t, containerKeyFor(cfg, serverBuildDir, cfg.Container.ImageName, cfg.Container.Tag))

	tag = "second"

	output, err := captureContainerStdout(t, func() error { return runBuild(buildCmd, nil) })
	if err != nil {
		t.Fatalf("runBuild() error = %v, want nil", err)
	}
	if strings.Contains(output, "(cached), skipping") {
		t.Errorf("--tag second reused the config-tag cache entry: %s", output)
	}
	if !strings.Contains(output, "Building container image") {
		t.Errorf("output = %q, want the rebuild to run", output)
	}
}

// TestRunBuildImageNameChangeInvalidates pins #692's image-name repro:
// changing container.imageName must produce a cache miss so image-two is
// actually built instead of skipping on image-one's warm entry.
func TestRunBuildImageNameChangeInvalidates(t *testing.T) {
	resetContainerFlags(t)
	cfg, serverBuildDir := containerBuildConfig(t)
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithNoLogs(true))
	stubBuildPrereqTools(t)

	seedContainerCache(t, containerKeyFor(cfg, serverBuildDir, "image-one", "latest"))

	cfg.Container.ImageName = "image-two"
	output, err := captureContainerStdout(t, func() error { return runBuild(buildCmd, nil) })
	if err != nil {
		t.Fatalf("runBuild() error = %v, want nil", err)
	}
	if strings.Contains(output, "(cached), skipping") {
		t.Errorf("image-two build reused image-one's cache entry: %s", output)
	}
}

// TestCheckBuildCacheOutputValidation pins #739 at the container command's
// skip seam: a hit is honored only while the built image still exists.
func TestCheckBuildCacheOutputValidation(t *testing.T) {
	const hash = "container-hash"

	tests := []struct {
		name   string
		output func() bool
		want   bool
	}{
		{name: "present image honors hit", output: func() bool { return true }, want: true},
		{name: "missing image rebuilds", output: func() bool { return false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seedContainerCache(t, hash)
			noCache = false
			t.Cleanup(func() { noCache = false })

			if got := checkBuildCache(cache.StageContainerBuild, hash, tt.output); got != tt.want {
				t.Errorf("checkBuildCache() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("no validator keeps current behavior", func(t *testing.T) {
		seedContainerCache(t, hash)
		if got := checkBuildCache(cache.StageContainerBuild, hash, nil); !got {
			t.Error("checkBuildCache() = false without a validator, want true")
		}
	})
}
