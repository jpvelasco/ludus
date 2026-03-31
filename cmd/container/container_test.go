package container

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devrecon/ludus/internal/cache"
)

var checkBuildCacheTests = []struct {
	name      string
	noCache   bool
	cacheData *cache.Cache // nil = no cache file
	stage     cache.StageKey
	hash      string
	want      bool
}{
	{
		name:    "noCache flag skips check",
		noCache: true,
		want:    false,
	},
	{
		name:  "missing cache file returns false",
		stage: cache.StageContainerBuild,
		hash:  "abc123",
		want:  false,
	},
	{
		name: "cache hit returns true",
		cacheData: &cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
			cache.StageContainerBuild: {Hash: "abc123", BuiltAt: "2026-01-01T00:00:00Z"},
		}},
		stage: cache.StageContainerBuild,
		hash:  "abc123",
		want:  true,
	},
	{
		name: "cache miss returns false",
		cacheData: &cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
			cache.StageContainerBuild: {Hash: "old-hash", BuiltAt: "2026-01-01T00:00:00Z"},
		}},
		stage: cache.StageContainerBuild,
		hash:  "new-hash",
		want:  false,
	},
	{
		name: "different stage returns false",
		cacheData: &cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
			cache.StageEngine: {Hash: "abc123", BuiltAt: "2026-01-01T00:00:00Z"},
		}},
		stage: cache.StageContainerBuild,
		hash:  "abc123",
		want:  false,
	},
}

func TestCheckBuildCache(t *testing.T) {
	for _, tt := range checkBuildCacheTests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)

			if tt.cacheData != nil {
				if err := os.MkdirAll(filepath.Join(dir, ".ludus"), 0o755); err != nil {
					t.Fatal(err)
				}
				data, _ := json.Marshal(tt.cacheData)
				if err := os.WriteFile(filepath.Join(dir, ".ludus", "cache.json"), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			origNoCache := noCache
			t.Cleanup(func() { noCache = origNoCache })
			noCache = tt.noCache

			got := checkBuildCache(tt.stage, tt.hash)
			if got != tt.want {
				t.Errorf("checkBuildCache() = %v, want %v", got, tt.want)
			}
		})
	}
}
