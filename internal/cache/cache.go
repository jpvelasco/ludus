// Package cache provides build caching for Ludus pipeline stages.
// Each stage computes a hash key from its inputs (git state, config values,
// file metadata). If the key matches a cached entry, the stage is skipped.
// Cache entries are stored in .ludus/cache.json.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	cacheDir  = ".ludus"
	cacheFile = "cache.json"
)

// StageKey identifies a pipeline stage for caching.
type StageKey string

const (
	StageEngine         StageKey = "engine"
	StageGameServer     StageKey = "game_server"
	StageGameClient     StageKey = "game_client"
	StageContainerBuild StageKey = "container_build"
)

// Entry is a single cache entry recording the input hash for a stage.
type Entry struct {
	Hash    string `json:"hash"`
	BuiltAt string `json:"builtAt"`
}

// Cache holds all stage cache entries.
type Cache struct {
	Entries map[StageKey]*Entry `json:"entries"`
}

func cachePath() string {
	return filepath.Join(cacheDir, cacheFile)
}

// Load reads .ludus/cache.json, returning an empty Cache if the file is missing.
func Load() (*Cache, error) {
	data, err := os.ReadFile(cachePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Cache{Entries: make(map[StageKey]*Entry)}, nil
		}
		return nil, err
	}

	c := &Cache{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}
	if c.Entries == nil {
		c.Entries = make(map[StageKey]*Entry)
	}
	return c, nil
}

// Save writes cache to .ludus/cache.json.
func Save(c *Cache) error {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cachePath(), data, 0644)
}

// IsHit returns true if the stage has a cached entry matching the given hash.
func (c *Cache) IsHit(stage StageKey, hash string) bool {
	entry, ok := c.Entries[stage]
	return ok && entry.Hash == hash
}

// MissReason explains why a cache check missed for a stage.
// Returns empty string if the cache is a hit (i.e., the stage is up to date).
func (c *Cache) MissReason(stage StageKey, hash string) string {
	entry, ok := c.Entries[stage]
	if !ok {
		return "no previous build recorded"
	}
	if entry.Hash != hash {
		return fmt.Sprintf("inputs changed since last build (%s)", entry.BuiltAt)
	}
	return ""
}

// Set updates the cache entry for a stage.
func (c *Cache) Set(stage StageKey, hash string, builtAt string) {
	c.Entries[stage] = &Entry{Hash: hash, BuiltAt: builtAt}
}

// CheckSkip loads the cache and returns true (with a skip message printed)
// if the stage is up to date. When noCache is true the check is bypassed.
// Returns false when the build should proceed.
func CheckSkip(stage StageKey, hash, projectName string, noCache bool) bool {
	if noCache {
		return false
	}
	c, err := Load()
	if err != nil {
		return false
	}
	if c.IsHit(stage, hash) {
		fmt.Printf("%s build is up to date (cached), skipping.\n", projectName)
		return true
	}
	if reason := c.MissReason(stage, hash); reason != "" {
		fmt.Printf("Cache: %s\n", reason)
	}
	return false
}

// RecordBuild updates the cache entry for a stage on success.
// A dry-run is side-effect free: it returns without recording an entry, so a
// subsequent real build is not skipped as "up to date (cached)".
func RecordBuild(stage StageKey, hash string, dryRun bool) {
	if dryRun {
		return
	}
	c, err := Load()
	if err != nil {
		return
	}
	c.Set(stage, hash, time.Now().UTC().Format(time.RFC3339))
	_ = Save(c)
}
