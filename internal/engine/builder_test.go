package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

// TestBuildSkipSetup verifies the SkipSetup gate: with an empty source tree,
// a normal build fails at the Setup step (Setup.sh/Setup.bat not found), while
// SkipSetup bypasses Setup entirely and fails at a later step instead — so the
// error must never be a setup failure. This is the #412 headless-Windows fix,
// where Setup.bat's redist installers hang in a non-interactive session.
func TestBuildSkipSetup(t *testing.T) {
	tests := []struct {
		name           string
		skipSetup      bool
		wantSetupError bool
	}{
		{"setup runs by default", false, true},
		{"setup skipped", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// DryRun runner so no real subprocess is spawned; the missing-file
			// checks in Setup/GenerateProjectFiles happen before the runner.
			r := runner.NewRunner(false, true)
			b := NewBuilder(BuildOptions{SourcePath: t.TempDir(), SkipSetup: tt.skipSetup}, r)

			result, err := b.Build(context.Background())
			if err == nil {
				t.Fatal("expected build to fail in an empty source tree")
			}
			gotSetupError := strings.Contains(err.Error(), "setup failed")
			if gotSetupError != tt.wantSetupError {
				t.Errorf("SkipSetup=%v: setup-error=%v, want %v (err: %v)",
					tt.skipSetup, gotSetupError, tt.wantSetupError, err)
			}
			if result == nil {
				t.Error("expected non-nil result even on failure")
			}
		})
	}
}
