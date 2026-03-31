package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devrecon/ludus/internal/config"
)

func TestCacheHitAndMiss(t *testing.T) {
	c := &Cache{Entries: make(map[StageKey]*Entry)}

	if c.IsHit(StageEngine, "abc123") {
		t.Fatal("expected cache miss for empty cache")
	}

	c.Set(StageEngine, "abc123", time.Now().UTC().Format(time.RFC3339))

	if !c.IsHit(StageEngine, "abc123") {
		t.Fatal("expected cache hit after Set")
	}

	if c.IsHit(StageEngine, "different") {
		t.Fatal("expected cache miss for different hash")
	}

	if c.IsHit(StageGameServer, "abc123") {
		t.Fatal("expected cache miss for different stage")
	}
}

func TestLoadSaveRoundtrip(t *testing.T) {
	// Work in a temp directory to avoid polluting the repo
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	c := &Cache{Entries: make(map[StageKey]*Entry)}
	c.Set(StageEngine, "hash1", "2025-01-01T00:00:00Z")
	c.Set(StageGameServer, "hash2", "2025-01-02T00:00:00Z")

	if err := Save(c); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !loaded.IsHit(StageEngine, "hash1") {
		t.Error("expected engine cache hit after roundtrip")
	}
	if !loaded.IsHit(StageGameServer, "hash2") {
		t.Error("expected game server cache hit after roundtrip")
	}
}

func TestLoadMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	c, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail on missing file: %v", err)
	}
	if len(c.Entries) != 0 {
		t.Fatalf("expected empty entries, got %d", len(c.Entries))
	}
}

func TestDirManifest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some files
	if err := os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("world!"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "sub", "c.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	m := dirManifest(tmpDir)
	if m == "" {
		t.Fatal("expected non-empty manifest")
	}

	// Manifest should be deterministic
	m2 := dirManifest(tmpDir)
	if m != m2 {
		t.Fatal("manifest not deterministic")
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Write corrupted JSON to the cache file
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath(), []byte("{not valid json!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = Load()
	if err == nil {
		t.Fatal("expected error loading corrupted cache file")
	}
}

func TestLoadNullEntries(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Write valid JSON but with null entries field
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath(), []byte(`{"entries": null}`), 0644); err != nil {
		t.Fatal(err)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// Should initialize nil entries map to empty
	if c.Entries == nil {
		t.Fatal("expected non-nil entries map after loading null entries")
	}
}

func TestMissReason(t *testing.T) {
	c := &Cache{Entries: make(map[StageKey]*Entry)}

	reason := c.MissReason(StageEngine, "abc")
	if reason != "no previous build recorded" {
		t.Errorf("expected 'no previous build recorded', got %q", reason)
	}

	c.Set(StageEngine, "abc", "2025-01-01T00:00:00Z")

	reason = c.MissReason(StageEngine, "abc")
	if reason != "" {
		t.Errorf("expected empty reason for hit, got %q", reason)
	}

	reason = c.MissReason(StageEngine, "different")
	if reason == "" {
		t.Error("expected non-empty reason for changed inputs")
	}
}

func TestHashDeterministic(t *testing.T) {
	h1 := hash("a", "b", "c")
	h2 := hash("a", "b", "c")
	if h1 != h2 {
		t.Fatal("hash not deterministic")
	}

	h3 := hash("a", "b", "d")
	if h1 == h3 {
		t.Fatal("different inputs should produce different hashes")
	}
}

func TestGitHEAD_NoRepo(t *testing.T) {
	tmpDir := t.TempDir()
	head := gitHEAD(tmpDir)
	if head != "" {
		t.Errorf("gitHEAD on non-repo should return empty string, got %q", head)
	}
}

func TestFileKey_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	nonexistent := filepath.Join(tmpDir, "does-not-exist.txt")
	key := fileKey(nonexistent)
	if key != "" {
		t.Errorf("fileKey on missing file should return empty string, got %q", key)
	}
}

func TestFileKey_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	content := []byte("hello world")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	key := fileKey(testFile)
	if key == "" {
		t.Fatal("fileKey should return non-empty string for existing file")
	}

	// Verify format contains ':' separator (mtime:size format)
	colonFound := false
	for _, c := range key {
		if c == ':' {
			colonFound = true
			break
		}
	}
	if !colonFound {
		t.Errorf("fileKey should contain ':' separator, got %q", key)
	}

	// Verify key ends with the correct size (11 bytes for "hello world")
	expectedSize := "11"
	if !endsWithAfterColon(key, expectedSize) {
		t.Errorf("fileKey should end with size %s, got %q", expectedSize, key)
	}
}

// endsWithAfterColon checks if the string ends with the expected suffix after the last colon.
func endsWithAfterColon(s, suffix string) bool {
	colonIdx := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			colonIdx = i
			break
		}
	}
	if colonIdx < 0 {
		return false
	}
	return s[colonIdx+1:] == suffix
}

func TestEngineKey_Deterministic(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:      "/fake/path",
			Version:         "5.7.3",
			MaxJobs:         8,
			Backend:         "native",
			DockerBaseImage: "ubuntu:22.04",
		},
	}

	key1 := EngineKey(cfg)
	key2 := EngineKey(cfg)

	if key1 != key2 {
		t.Error("EngineKey should be deterministic for same config")
	}
	if key1 == "" {
		t.Error("EngineKey should return non-empty string")
	}
}

func TestGameServerKey_DifferentArchDifferentKey(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "/fake/engine",
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectPath:  "/fake/project.uproject",
			ProjectName:  "TestGame",
			ServerTarget: "TestGameServer",
			GameTarget:   "TestGame",
			ServerMap:    "/Game/Maps/TestMap",
			Arch:         "amd64",
		},
	}

	engineHash := "abc123"

	keyAmd64 := GameServerKey(cfg, engineHash)

	// Change to arm64
	cfg.Game.Arch = "arm64"
	keyArm64 := GameServerKey(cfg, engineHash)

	if keyAmd64 == keyArm64 {
		t.Error("GameServerKey should produce different keys for different architectures")
	}
	if keyAmd64 == "" || keyArm64 == "" {
		t.Error("GameServerKey should return non-empty strings")
	}
}

func TestContainerKey_DifferentPort(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake server build directory
	if err := os.WriteFile(filepath.Join(tmpDir, "test.bin"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName:  "TestGame",
			ServerTarget: "TestGameServer",
		},
		Container: config.ContainerConfig{
			ServerPort: 7777,
			Tag:        "latest",
		},
	}

	key1 := ContainerKey(cfg, tmpDir)

	// Change port
	cfg.Container.ServerPort = 8888
	key2 := ContainerKey(cfg, tmpDir)

	if key1 == key2 {
		t.Error("ContainerKey should produce different keys for different ports")
	}
	if key1 == "" || key2 == "" {
		t.Error("ContainerKey should return non-empty strings")
	}
}

func TestCheckSkip_CacheHit(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Seed the cache with a known entry
	c := &Cache{Entries: make(map[StageKey]*Entry)}
	c.Set(StageEngine, "match-hash", "2025-06-01T00:00:00Z")
	if err := Save(c); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Should return true (skip) when hash matches
	if !CheckSkip(StageEngine, "match-hash", "TestProject", false) {
		t.Error("expected CheckSkip to return true for cache hit")
	}
}

func TestCheckSkip_CacheMiss(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Seed the cache with a different hash
	c := &Cache{Entries: make(map[StageKey]*Entry)}
	c.Set(StageEngine, "old-hash", "2025-06-01T00:00:00Z")
	if err := Save(c); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Should return false (proceed) when hash differs
	if CheckSkip(StageEngine, "new-hash", "TestProject", false) {
		t.Error("expected CheckSkip to return false for cache miss")
	}
}

func TestCheckSkip_NoCache(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Seed the cache with a matching hash
	c := &Cache{Entries: make(map[StageKey]*Entry)}
	c.Set(StageEngine, "match-hash", "2025-06-01T00:00:00Z")
	if err := Save(c); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Should return false (proceed) when noCache is true, even with matching hash
	if CheckSkip(StageEngine, "match-hash", "TestProject", true) {
		t.Error("expected CheckSkip to return false when noCache is true")
	}
}

func TestCheckSkip_EmptyCache(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// No cache file exists at all
	if CheckSkip(StageEngine, "any-hash", "TestProject", false) {
		t.Error("expected CheckSkip to return false for empty cache")
	}
}

func TestRecordBuild(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Record a build
	RecordBuild(StageGameServer, "server-hash-123")

	// Verify the entry was saved
	c, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !c.IsHit(StageGameServer, "server-hash-123") {
		t.Error("expected cache hit after RecordBuild")
	}
	entry := c.Entries[StageGameServer]
	if entry.BuiltAt == "" {
		t.Error("expected BuiltAt to be set")
	}
}

func TestRecordBuild_OverwritesPrevious(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Record first build
	RecordBuild(StageEngine, "hash-v1")

	// Record second build for same stage
	RecordBuild(StageEngine, "hash-v2")

	// Verify only the latest entry exists
	c, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if c.IsHit(StageEngine, "hash-v1") {
		t.Error("expected old hash to be replaced")
	}
	if !c.IsHit(StageEngine, "hash-v2") {
		t.Error("expected new hash to be recorded")
	}
}

func TestRecordBuild_MultipleStages(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	RecordBuild(StageEngine, "engine-hash")
	RecordBuild(StageGameServer, "server-hash")
	RecordBuild(StageContainerBuild, "container-hash")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !c.IsHit(StageEngine, "engine-hash") {
		t.Error("expected engine cache hit")
	}
	if !c.IsHit(StageGameServer, "server-hash") {
		t.Error("expected game server cache hit")
	}
	if !c.IsHit(StageContainerBuild, "container-hash") {
		t.Error("expected container build cache hit")
	}
}
