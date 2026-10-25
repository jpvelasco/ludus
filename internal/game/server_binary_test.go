package game

import (
	"os"
	"path/filepath"
	"testing"
)

func writeServerBinaries(t *testing.T, buildDir, platformDir string, names []string) {
	t.Helper()
	binDir := filepath.Join(buildDir, "Lyra", "Binaries", platformDir)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("creating binary dir: %v", err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("binary"), 0755); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
}

// TestResolveServerBinaryName covers the canonical packaged-server discovery
// shared by the container, EC2, and Anywhere deployers: bare Development
// names, suffixed Shipping/Test names, precedence when both exist, and the
// bare fallback when the directory is missing or empty (#744).
func TestResolveServerBinaryName(t *testing.T) {
	tests := []struct {
		name             string
		files            []string
		platformDir      string
		executableSuffix string
		want             string
	}{
		{
			name:        "bare development binary",
			files:       []string{"LyraServer"},
			platformDir: "Linux",
			want:        "LyraServer",
		},
		{
			name:        "shipping suffix wins over bare binary",
			files:       []string{"LyraServer", "LyraServer-Linux-Shipping"},
			platformDir: "Linux",
			want:        "LyraServer-Linux-Shipping",
		},
		{
			name:        "test suffix on arm64",
			files:       []string{"LyraServer-LinuxArm64-Test"},
			platformDir: "LinuxArm64",
			want:        "LyraServer-LinuxArm64-Test",
		},
		{
			name:        "dot-bearing decoys are skipped",
			files:       []string{"LyraServer-Linux-Shipping.debug", "LyraServer-Linux-Shipping.sym"},
			platformDir: "Linux",
			want:        "LyraServer",
		},
		{
			name:        "unconventional config segment still matches",
			files:       []string{"LyraServer-Linux-DebugData"},
			platformDir: "Linux",
			want:        "LyraServer-Linux-DebugData",
		},
		{
			name:             "windows exe suffix",
			files:            []string{"LyraServer-Win64-Shipping.exe"},
			platformDir:      "Win64",
			executableSuffix: ".exe",
			want:             "LyraServer-Win64-Shipping.exe",
		},
		{
			name:             "windows entry without exe suffix is skipped",
			files:            []string{"LyraServer-Win64-Shipping"},
			platformDir:      "Win64",
			executableSuffix: ".exe",
			want:             "LyraServer.exe",
		},
		{
			name:        "no binaries present falls back to bare target",
			files:       nil,
			platformDir: "Linux",
			want:        "LyraServer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buildDir := t.TempDir()
			if tt.files != nil {
				writeServerBinaries(t, buildDir, tt.platformDir, tt.files)
			}
			got := ResolveServerBinaryName(buildDir, "Lyra", tt.platformDir, "LyraServer", tt.executableSuffix)
			if got != tt.want {
				t.Errorf("ResolveServerBinaryName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveServerBinaryNameEmptyBuildDir pins the guard that keeps an
// empty build dir from reading a working-directory-relative Binaries path.
func TestResolveServerBinaryNameEmptyBuildDir(t *testing.T) {
	got := ResolveServerBinaryName("", "Lyra", "Linux", "LyraServer", "")
	if got != "LyraServer" {
		t.Errorf("ResolveServerBinaryName(empty dir) = %q, want bare LyraServer", got)
	}
}

// TestResolveServerBinaryNameMissingDir pins that a not-yet-packaged build
// directory degrades to the bare target name.
func TestResolveServerBinaryNameMissingDir(t *testing.T) {
	got := ResolveServerBinaryName(t.TempDir(), "Lyra", "Linux", "LyraServer", "")
	if got != "LyraServer" {
		t.Errorf("ResolveServerBinaryName(missing dir) = %q, want bare LyraServer", got)
	}
}
