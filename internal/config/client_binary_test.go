package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFiles creates each named file (empty) under dir.
func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatalf("writing %s: %v", n, err)
		}
	}
}

func TestDiscoverClientBinary_LinuxFindsRealTarget(t *testing.T) {
	// #395: the staged binary is LyraGame, but the conventional fallback would be
	// LyraStarterGameGame. Discovery must return the actual file on disk,
	// ignoring the UE sidecars UE stages alongside it (debug symbols, build
	// receipts, and module manifests all contain a dot; the executable does not).
	dir := t.TempDir()
	writeFiles(t, dir,
		"LyraGame",
		"LyraGame.debug",
		"LyraGame.sym",
		"LyraGame.target",  // UE build receipt
		"LyraGame.modules", // UE module manifest
		"libfoo.so",
	)
	fallback := filepath.Join(dir, "LyraStarterGameGame")

	got := DiscoverClientBinary(dir, fallback, false)
	want := filepath.Join(dir, "LyraGame")
	if got != want {
		t.Errorf("DiscoverClientBinary() = %q, want %q", got, want)
	}
}

func TestDiscoverClientBinary_WindowsPrefersExe(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "LyraGame.exe", "LyraGame.pdb")
	fallback := filepath.Join(dir, "LyraStarterGameGame.exe")

	got := DiscoverClientBinary(dir, fallback, true)
	want := filepath.Join(dir, "LyraGame.exe")
	if got != want {
		t.Errorf("DiscoverClientBinary() = %q, want %q", got, want)
	}
}

func TestDiscoverClientBinary_Fallback(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{"missing directory", func(t *testing.T, dir string) {
			// Point at a subpath that does not exist.
		}},
		{"ambiguous: two executables", func(t *testing.T, dir string) {
			writeFiles(t, dir, "LyraGame", "ExtraTool")
		}},
		{"empty: only symbols", func(t *testing.T, dir string) {
			writeFiles(t, dir, "LyraGame.sym", "LyraGame.debug")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			scanDir := dir
			if tt.name == "missing directory" {
				scanDir = filepath.Join(dir, "nope")
			}
			tt.setup(t, dir)
			fallback := filepath.Join(scanDir, "Fallback")
			if got := DiscoverClientBinary(scanDir, fallback, false); got != fallback {
				t.Errorf("DiscoverClientBinary() = %q, want fallback %q", got, fallback)
			}
		})
	}
}
