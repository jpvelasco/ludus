package cache

import (
	"os"
	"testing"
)

func TestLoadSaveRoundtrip(t *testing.T) {
	t.Chdir(t.TempDir())

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
	t.Chdir(t.TempDir())

	c, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail on missing file: %v", err)
	}
	if len(c.Entries) != 0 {
		t.Fatalf("expected empty entries, got %d", len(c.Entries))
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath(), []byte("{not valid json!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected error loading corrupted cache file")
	}
}

func TestLoadNullEntries(t *testing.T) {
	t.Chdir(t.TempDir())

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
	if c.Entries == nil {
		t.Fatal("expected non-nil entries map after loading null entries")
	}
}
