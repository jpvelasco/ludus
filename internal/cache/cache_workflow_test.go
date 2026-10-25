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

			got := CheckSkip(StageEngine, tt.checkHash, "TestProject", tt.noCache, nil)
			if got != tt.wantSkip {
				t.Errorf("CheckSkip = %v, want %v", got, tt.wantSkip)
			}
		})
	}
}

// recordAndLoad runs RecordBuild for the given stage/hash pairs and returns
// the reloaded cache plus any load error.
func recordAndLoad(recordings [][2]any) (*Cache, error) {
	for _, rec := range recordings {
		RecordBuild(rec[0].(StageKey), rec[1].(string), false)
	}
	return Load()
}

// assertStageHits asserts that every listed stage/hash pair is a hit and every
// pair in gone is not.
func assertStageHits(t *testing.T, c *Cache, want, gone map[StageKey]string) {
	t.Helper()
	for stage, hash := range want {
		if !c.IsHit(stage, hash) {
			t.Errorf("expected %s cache hit for %q", stage, hash)
		}
	}
	for stage, hash := range gone {
		if c.IsHit(stage, hash) {
			t.Errorf("expected %s miss for %q", stage, hash)
		}
	}
}

func TestRecordBuild(t *testing.T) {
	tests := []struct {
		name      string
		record    [][2]any
		want      map[StageKey]string
		gone      map[StageKey]string
		checkTime bool
	}{
		{
			name:      "records entry",
			record:    [][2]any{{StageGameServer, "server-hash-123"}},
			want:      map[StageKey]string{StageGameServer: "server-hash-123"},
			checkTime: true,
		},
		{
			name:   "overwrites previous",
			record: [][2]any{{StageEngine, "hash-v1"}, {StageEngine, "hash-v2"}},
			want:   map[StageKey]string{StageEngine: "hash-v2"},
			gone:   map[StageKey]string{StageEngine: "hash-v1"},
		},
		{
			name: "multiple stages",
			record: [][2]any{
				{StageEngine, "engine-hash"},
				{StageGameServer, "server-hash"},
				{StageContainerBuild, "container-hash"},
			},
			want: map[StageKey]string{
				StageEngine:         "engine-hash",
				StageGameServer:     "server-hash",
				StageContainerBuild: "container-hash",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			c, err := recordAndLoad(tt.record)
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}
			assertStageHits(t, c, tt.want, tt.gone)
			if tt.checkTime && c.Entries[StageGameServer].BuiltAt == "" {
				t.Error("expected BuiltAt to be set")
			}
		})
	}
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
	if CheckSkip(StageEngine, "engine-hash", "Engine", false, nil) {
		t.Fatal("dry-run poisoned the cache: real build would be skipped as cached")
	}

	// 2. Real run: records the entry as normal.
	RecordBuild(StageEngine, "engine-hash", false)
	if !CheckSkip(StageEngine, "engine-hash", "Engine", false, nil) {
		t.Error("real build was not recorded after dry-run")
	}
}
