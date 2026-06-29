//go:build windows

package prereq

import (
	"slices"
	"testing"
)

// findPluginFix returns the knownPluginDLLFixes entry with the given name.
func findPluginFix(t *testing.T, name string) pluginDLLFix {
	t.Helper()
	for _, f := range knownPluginDLLFixes {
		if f.name == name {
			return f
		}
	}
	t.Fatalf("plugin fix %q not found", name)
	return pluginDLLFix{}
}

// TestPluginFixVersionGates pins the version gates for the known plugin DLL
// fixes. PlatformCrypto must cover both 5.7 and 5.8 (the plugin lives at the
// same path on 5.8, and omitting 8 would make cleanupStaleDLLs delete the
// needed DLLs); Dataflow must remain 5.6-only (Epic moved it into engine
// binaries natively from 5.7, so copying it causes class conflicts).
func TestPluginFixVersionGates(t *testing.T) {
	tests := []struct {
		fixName      string
		wantVersions []int
	}{
		{"PlatformCrypto Plugin DLLs", []int{7, 8}},
		{"Dataflow Plugin DLLs", []int{6}},
	}

	for _, tt := range tests {
		t.Run(tt.fixName, func(t *testing.T) {
			fix := findPluginFix(t, tt.fixName)
			if !slices.Equal(fix.minorVersions, tt.wantVersions) {
				t.Errorf("%s minorVersions = %v, want %v", tt.fixName, fix.minorVersions, tt.wantVersions)
			}
		})
	}
}
