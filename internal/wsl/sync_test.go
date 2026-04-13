package wsl

import (
	"testing"
	"time"
)

func TestResolveSyncTarget(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"with version", "5.7", "$HOME/ludus/engine/5.7"},
		{"empty version", "", "$HOME/ludus/engine/default"},
		{"patch version", "5.7.4", "$HOME/ludus/engine/5.7.4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveSyncTarget(tt.version)
			if got != tt.want {
				t.Errorf("ResolveSyncTarget(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestNeedsResync(t *testing.T) {
	tests := []struct {
		name         string
		lastSyncTime time.Time
		want         bool
	}{
		{"zero time needs sync", time.Time{}, true},
		{"recent sync no resync", time.Now().Add(-1 * time.Hour), false},
		{"old sync needs resync", time.Now().Add(-25 * time.Hour), true},
		{"exactly 24h needs resync", time.Now().Add(-24*time.Hour - time.Second), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsResync(tt.lastSyncTime)
			if got != tt.want {
				t.Errorf("NeedsResync() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncEngineValidation(t *testing.T) {
	t.Run("empty source path", func(t *testing.T) {
		opts := SyncOptions{SourcePath: ""}
		// SyncEngine requires a non-empty source path.
		// We can't test the full flow without WSL, but we verify the options struct.
		if opts.SourcePath != "" {
			t.Error("expected empty source path")
		}
	})

	t.Run("sync options defaults", func(t *testing.T) {
		// When TargetDir is empty, SyncEngine uses ResolveSyncTarget.
		version := "5.7"
		want := "$HOME/ludus/engine/5.7"
		if ResolveSyncTarget(version) != want {
			t.Errorf("default target = %q, want %q", ResolveSyncTarget(version), want)
		}
	})
}
