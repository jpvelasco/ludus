package prereq

import (
	"testing"
)

// TestFixAppliesToMinor pins the version gates consulted by manifest-based
// stale cleanup: a fix owns its copies on the minors it covers, unknown fixes
// own nothing on any version (#746).
func TestFixAppliesToMinor(t *testing.T) {
	tests := []struct {
		fixName string
		minor   int
		want    bool
	}{
		{"Dataflow Plugin DLLs", 6, true},
		{"Dataflow Plugin DLLs", 7, false},
		{"Dataflow Plugin DLLs", 8, false},
		{"PlatformCrypto Plugin DLLs", 7, true},
		{"PlatformCrypto Plugin DLLs", 8, true},
		{"PlatformCrypto Plugin DLLs", 6, false},
		{"Unknown Fix", 6, false},
		{"Unknown Fix", 7, false},
	}
	for _, tt := range tests {
		if got := fixAppliesToMinor(tt.fixName, tt.minor); got != tt.want {
			t.Errorf("fixAppliesToMinor(%q, %d) = %v, want %v", tt.fixName, tt.minor, got, tt.want)
		}
	}
}

// TestSelectStaleCopies pins the #746 selection rule: only copies recorded in
// the provenance manifest whose owning fix no longer applies to the current
// engine minor are stale. Native engine binaries that merely share a
// workaround DLL's filename never appear here, because ludus never copied
// them.
func TestSelectStaleCopies(t *testing.T) {
	tests := []struct {
		name   string
		copies []pluginDLLCopy
		minor  int
		want   []string
	}{
		{
			name:  "no manifest entries means nothing stale",
			minor: 7,
		},
		{
			name: "5.6 dataflow copies stay owned on 5.6",
			copies: []pluginDLLCopy{
				{Fix: "Dataflow Plugin DLLs", File: "UnrealEditor-DataflowEditor.dll"},
			},
			minor: 6,
		},
		{
			name: "5.6 dataflow copies are stale on 5.7 and 5.8",
			copies: []pluginDLLCopy{
				{Fix: "Dataflow Plugin DLLs", File: "UnrealEditor-DataflowEditor.dll"},
				{Fix: "Dataflow Plugin DLLs", File: "UnrealEditor-DataflowNodes.dll"},
			},
			minor: 7,
			want:  []string{"UnrealEditor-DataflowEditor.dll", "UnrealEditor-DataflowNodes.dll"},
		},
		{
			name: "platform crypto copies stay owned on 5.8",
			copies: []pluginDLLCopy{
				{Fix: "PlatformCrypto Plugin DLLs", File: "UnrealEditor-PlatformCrypto.dll"},
			},
			minor: 8,
		},
		{
			name: "unknown fix copies are stale on every version",
			copies: []pluginDLLCopy{
				{Fix: "Removed Fix", File: "UnrealEditor-Removed.dll"},
			},
			minor: 6,
			want:  []string{"UnrealEditor-Removed.dll"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stale := selectStaleCopies(tt.copies, tt.minor)
			if len(stale) != len(tt.want) {
				t.Fatalf("selectStaleCopies() = %#v, want %d entries", stale, len(tt.want))
			}
			for i, want := range tt.want {
				if stale[i].File != want {
					t.Errorf("stale[%d] = %q, want %q", i, stale[i].File, want)
				}
			}
		})
	}
}

// TestRemainingCopies pins the manifest rewrite after removal: entries whose
// files were removed drop out, while entries that failed to remove (or still
// apply) must survive so a later run reconsiders them (#746).
func TestRemainingCopies(t *testing.T) {
	manifest := []pluginDLLCopy{
		{Fix: "Dataflow Plugin DLLs", File: "a.dll"},
		{Fix: "Dataflow Plugin DLLs", File: "b.dll"},
		{Fix: "PlatformCrypto Plugin DLLs", File: "c.dll"},
	}
	stale := []pluginDLLCopy{
		{Fix: "Dataflow Plugin DLLs", File: "a.dll"},
		{Fix: "Dataflow Plugin DLLs", File: "b.dll"},
	}
	removed := []string{"a.dll"}

	want := []pluginDLLCopy{
		{Fix: "Dataflow Plugin DLLs", File: "b.dll"},
		{Fix: "PlatformCrypto Plugin DLLs", File: "c.dll"},
	}
	got := remainingCopies(manifest, stale, removed)
	if len(got) != len(want) {
		t.Fatalf("remainingCopies() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("remainingCopies()[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

// TestFilenameCleanupMinor pins that filename-based stale cleanup is restricted
// to the engine minor where the workaround actually applied: on 5.7+ the same
// Dataflow names belong to native engine binaries and must never be deleted
// based on the table alone (#746).
func TestFilenameCleanupMinor(t *testing.T) {
	tests := []struct {
		minor int
		want  bool
	}{
		{5, false},
		{6, true},
		{7, false},
		{8, false},
	}
	for _, tt := range tests {
		if got := tt.minor == filenameCleanupMinor; got != tt.want {
			t.Errorf("minor %d filename fallback = %v, want %v", tt.minor, got, tt.want)
		}
	}
}
