// Package cache provides build caching for Ludus pipeline stages.
// Each stage computes a hash key from its inputs (git state, config values,
// file metadata). If the key matches a cached entry, the stage is skipped.
// Cache entries are stored in .ludus/cache.json.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/devrecon/ludus/internal/config"
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

// Set updates the cache entry for a stage.
func (c *Cache) Set(stage StageKey, hash string, builtAt string) {
	c.Entries[stage] = &Entry{Hash: hash, BuiltAt: builtAt}
}

// hash computes a SHA-256 hex digest from a list of key-value strings.
func hash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0}) // separator
	}
	return hex.EncodeToString(h.Sum(nil))
}

// gitHEAD returns the git HEAD commit hash for the given directory.
// Returns empty string if git is not available or the directory is not a repo.
func gitHEAD(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// fileKey returns "mtime:size" for a file, or empty string if stat fails.
func fileKey(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
}

// EngineKey computes the cache key for the engine build stage.
func EngineKey(cfg *config.Config) string {
	return hash(
		gitHEAD(cfg.Engine.SourcePath),
		cfg.Engine.Version,
		fmt.Sprintf("%d", cfg.Engine.MaxJobs),
		runtime.GOOS,
		cfg.Engine.Backend,
		cfg.Engine.DockerBaseImage,
	)
}

// GameServerKey computes the cache key for the game server build stage.
func GameServerKey(cfg *config.Config, engineHash string) string {
	projectPath := cfg.Game.ProjectPath
	if projectPath == "" && cfg.Game.ProjectName == "Lyra" {
		projectPath = filepath.Join(cfg.Engine.SourcePath,
			"Samples", "Games", "Lyra", "Lyra.uproject")
	}

	return hash(
		engineHash,
		fileKey(projectPath),
		cfg.Game.ResolvedServerTarget(),
		cfg.Game.ResolvedGameTarget(),
		cfg.Game.ServerMap,
		fmt.Sprintf("%v", cfg.Game.SkipCook),
		cfg.Engine.Version,
	)
}

// GameClientKey computes the cache key for the game client build stage.
func GameClientKey(cfg *config.Config, engineHash string, platform string) string {
	projectPath := cfg.Game.ProjectPath
	if projectPath == "" && cfg.Game.ProjectName == "Lyra" {
		projectPath = filepath.Join(cfg.Engine.SourcePath,
			"Samples", "Games", "Lyra", "Lyra.uproject")
	}

	return hash(
		engineHash,
		fileKey(projectPath),
		cfg.Game.ResolvedClientTarget(),
		platform,
		fmt.Sprintf("%v", cfg.Game.SkipCook),
		cfg.Engine.Version,
	)
}

// ContainerKey computes the cache key for the container build stage.
// It hashes a manifest of filenames and sizes in the server build directory.
func ContainerKey(cfg *config.Config, serverBuildDir string) string {
	manifest := dirManifest(serverBuildDir)
	return hash(
		manifest,
		cfg.Game.ProjectName,
		cfg.Game.ResolvedServerTarget(),
		fmt.Sprintf("%d", cfg.Container.ServerPort),
		cfg.Container.Tag,
	)
}

// dirManifest returns a deterministic string of "name:size" entries
// for all files in a directory tree, sorted by path.
func dirManifest(dir string) string {
	var entries []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		entries = append(entries, fmt.Sprintf("%s:%d", rel, info.Size()))
		return nil
	})
	sort.Strings(entries)
	return strings.Join(entries, "\n")
}
