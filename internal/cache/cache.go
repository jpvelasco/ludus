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
	"sync"
	"time"
)

// cacheMu serializes Load/Save/RecordBuild so concurrent MCP builds cannot
// truncate cache.json or lose a sibling stage's update.
var cacheMu sync.Mutex

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

// New returns an empty, ready-to-use cache (non-nil Entries map).
func New() *Cache {
	return &Cache{Entries: make(map[StageKey]*Entry)}
}

// Load reads .ludus/cache.json, returning an empty Cache if the file is missing.
func Load() (*Cache, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	return loadUnlocked()
}

func loadUnlocked() (*Cache, error) {
	data, err := os.ReadFile(cachePath())
	if err != nil {
		if os.IsNotExist(err) {
			return New(), nil
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

// Save writes cache to .ludus/cache.json atomically (temp file + rename).
func Save(c *Cache) error {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	return saveUnlocked(c)
}

func saveUnlocked(c *Cache) error {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	p := cachePath()
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replacing %s: %w", p, err)
	}
	return nil
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

// OutputExists reports whether a stage's output artifact is still present.
// nil means the caller has no cheaply verifiable output, in which case a
// matching hash alone honors the hit (the pre-#739 behavior).
type OutputExists func() bool

// OutputMissingMessage is printed when a cache hit is discarded because the
// stage's output artifact no longer exists, so the user understands why the
// rebuild ran (#739).
const OutputMissingMessage = "cache hit ignored: output missing"

// UsableHit reports whether the cached entry for stage can be honored: the
// recorded hash must match and, when output is non-nil, the stage's output
// artifact must still exist. A matching entry whose output is missing prints
// OutputMissingMessage and is treated as a miss so the stage reruns and
// recreates it (#739).
func (c *Cache) UsableHit(stage StageKey, hash string, output OutputExists) bool {
	if !c.IsHit(stage, hash) {
		return false
	}
	if output != nil && !output() {
		fmt.Println(OutputMissingMessage)
		return false
	}
	return true
}

// DirExists returns an OutputExists that reports whether path is a directory.
// An empty path yields nil: the caller has no output path to verify, so hits
// are accepted on the hash alone (#739).
func DirExists(path string) OutputExists {
	if path == "" {
		return nil
	}
	return func() bool {
		info, err := os.Stat(path)
		return err == nil && info.IsDir()
	}
}

// Set updates the cache entry for a stage.
func (c *Cache) Set(stage StageKey, hash string, builtAt string) {
	c.Entries[stage] = &Entry{Hash: hash, BuiltAt: builtAt}
}

// CheckSkip loads the cache and returns true (with a skip message printed)
// if the stage is up to date. When noCache is true the check is bypassed.
// output, when non-nil, must also report the stage's output artifact present
// for the hit to be honored; a hit with missing output is discarded with
// OutputMissingMessage so the build recreates it (#739).
// Returns false when the build should proceed.
func CheckSkip(stage StageKey, hash, projectName string, noCache bool, output OutputExists) bool {
	if noCache {
		return false
	}
	c, err := Load()
	if err != nil {
		return false
	}
	if c.UsableHit(stage, hash, output) {
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
	cacheMu.Lock()
	defer cacheMu.Unlock()
	c, err := loadUnlocked()
	if err != nil {
		return
	}
	c.Set(stage, hash, time.Now().UTC().Format(time.RFC3339))
	_ = saveUnlocked(c)
}
