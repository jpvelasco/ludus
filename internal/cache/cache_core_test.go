package cache

import (
	"testing"
	"time"
)

func TestIsHit(t *testing.T) {
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

func TestMissReason(t *testing.T) {
	c := &Cache{Entries: make(map[StageKey]*Entry)}

	if got := c.MissReason(StageEngine, "abc"); got != "no previous build recorded" {
		t.Errorf("expected 'no previous build recorded', got %q", got)
	}

	c.Set(StageEngine, "abc", "2025-01-01T00:00:00Z")

	if got := c.MissReason(StageEngine, "abc"); got != "" {
		t.Errorf("expected empty reason for hit, got %q", got)
	}
	if got := c.MissReason(StageEngine, "different"); got == "" {
		t.Error("expected non-empty reason for changed inputs")
	}
}
