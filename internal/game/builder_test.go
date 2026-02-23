package game

import (
	"testing"

	"github.com/devrecon/ludus/internal/runner"
)

func TestApplyNuGetAuditWorkaround(t *testing.T) {
	tests := []struct {
		name          string
		engineVersion string
		wantApplied   bool
	}{
		{"5.6 applies", "5.6", true},
		{"empty applies (safe default)", "", true},
		{"5.5 skips", "5.5", false},
		{"5.7 skips", "5.7", false},
		{"5.4 skips", "5.4", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runner.NewRunner(false, true) // dry-run mode
			b := NewBuilder(BuildOptions{
				EngineVersion: tt.engineVersion,
			}, r)

			b.applyNuGetAuditWorkaround()

			found := false
			for _, kv := range b.Runner.Env {
				if kv == "NuGetAuditLevel=critical" {
					found = true
					break
				}
			}

			if found != tt.wantApplied {
				t.Errorf("applyNuGetAuditWorkaround() applied=%v, want %v (engineVersion=%q)",
					found, tt.wantApplied, tt.engineVersion)
			}
		})
	}
}
