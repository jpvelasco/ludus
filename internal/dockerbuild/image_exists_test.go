package dockerbuild

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

// TestImageExists pins the #603 guard used before honoring container-stage
// cache hits: an inspect that succeeds reports existence, anything else
// (missing image, unreachable daemon) reports false.
func TestImageExists(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		dryRun   bool
		expected bool
	}{
		{
			name:     "dry-run runner reports existing",
			cli:      "docker",
			dryRun:   true,
			expected: true,
		},
		{
			name:     "missing binary reports missing",
			cli:      "ludus-nonexistent-cli-xyz",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runner.NewRunner(false, tt.dryRun)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			got := ImageExists(r, ctx, tt.cli, "ludus-engine:5.8.1")
			if got != tt.expected {
				t.Errorf("ImageExists(%q) = %v, want %v", tt.cli, got, tt.expected)
			}
		})
	}
}
