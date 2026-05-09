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

		RecordBuild(StageGameServer, "server-hash-123")

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

		RecordBuild(StageEngine, "hash-v1")
		RecordBuild(StageEngine, "hash-v2")

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
	})
}
