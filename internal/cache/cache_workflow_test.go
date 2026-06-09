package cache

import (
	"testing"
)

func TestCheckSkip(t *testing.T) {
	tests := []struct {
		name      string
		seedHash  string // empty means no seed
		checkHash string
		noCache   bool
		wantSkip  bool
	}{
		{
			name:      "hit",
			seedHash:  "match-hash",
			checkHash: "match-hash",
			wantSkip:  true,
		},
		{
			name:      "miss changed hash",
			seedHash:  "old-hash",
			checkHash: "new-hash",
			wantSkip:  false,
		},
		{
			name:      "no cache flag overrides hit",
			seedHash:  "match-hash",
			checkHash: "match-hash",
			noCache:   true,
			wantSkip:  false,
		},
		{
			name:      "empty cache",
			checkHash: "any-hash",
			wantSkip:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			if tt.seedHash != "" {
				c := &Cache{Entries: make(map[StageKey]*Entry)}
				c.Set(StageEngine, tt.seedHash, "2025-06-01T00:00:00Z")
				if err := Save(c); err != nil {
					t.Fatalf("Save failed: %v", err)
				}
			}

			got := CheckSkip(StageEngine, tt.checkHash, "TestProject", tt.noCache)
			if got != tt.wantSkip {
				t.Errorf("CheckSkip = %v, want %v", got, tt.wantSkip)
			}
		})
	}
}

func TestRecordBuild(t *testing.T) {
	t.Run("records entry", func(t *testing.T) {
		t.Chdir(t.TempDir())

		RecordBuild(StageGameServer, "server-hash-123", false)

		c, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if !c.IsHit(StageGameServer, "server-hash-123") {
			t.Error("expected cache hit after RecordBuild")
		}
		if c.Entries[StageGameServer].BuiltAt == "" {
			t.Error("expected BuiltAt to be set")
		}
	})

	t.Run("overwrites previous", func(t *testing.T) {
		t.Chdir(t.TempDir())

		RecordBuild(StageEngine, "hash-v1", false)
		RecordBuild(StageEngine, "hash-v2", false)

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
	})

	t.Run("multiple stages", func(t *testing.T) {
		t.Chdir(t.TempDir())

		RecordBuild(StageEngine, "engine-hash", false)
		RecordBuild(StageGameServer, "server-hash", false)
		RecordBuild(StageContainerBuild, "container-hash", false)

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
	})
}

// TestRecordBuildDryRun verifies that a dry-run is side-effect free: it must not
// write a cache entry, so a subsequent real build is not skipped as cached (#273).
func TestRecordBuildDryRun(t *testing.T) {
	tests := []struct {
		name       string
		dryRun     bool
		wantRecord bool
	}{
		{name: "dry-run does not record", dryRun: true, wantRecord: false},
		{name: "real run records", dryRun: false, wantRecord: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			RecordBuild(StageGameServer, "hash-abc", tt.dryRun)

			c, err := Load()
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}
			if got := c.IsHit(StageGameServer, "hash-abc"); got != tt.wantRecord {
				t.Errorf("after RecordBuild(dryRun=%v): IsHit = %v, want %v", tt.dryRun, got, tt.wantRecord)
			}
		})
	}
}

// TestRecordBuildDryRunDoesNotPoison reproduces the #273 sequence end-to-end:
// a dry-run followed by a real run must leave the real build's entry recorded
// (the dry-run must not pre-seed a hit that causes the real build to be skipped).
func TestRecordBuildDryRunDoesNotPoison(t *testing.T) {
	t.Chdir(t.TempDir())

	// 1. Dry-run: previews the build, must NOT touch the cache.
	RecordBuild(StageEngine, "engine-hash", true)
	if CheckSkip(StageEngine, "engine-hash", "Engine", false) {
		t.Fatal("dry-run poisoned the cache: real build would be skipped as cached")
	}

	// 2. Real run: records the entry as normal.
	RecordBuild(StageEngine, "engine-hash", false)
	if !CheckSkip(StageEngine, "engine-hash", "Engine", false) {
		t.Error("real build was not recorded after dry-run")
	}
}
