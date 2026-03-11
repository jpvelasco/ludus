package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
