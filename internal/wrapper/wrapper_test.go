package wrapper

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheDir(t *testing.T) {
	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error: %v", err)
	}
	if dir == "" {
		t.Fatal("CacheDir() returned empty string")
	}

	// Verify path ends with expected suffix (platform-agnostic)
	expectedSuffix := filepath.Join(".cache", "ludus", "game-server-wrapper")
	if !strings.HasSuffix(dir, expectedSuffix) {
		t.Errorf("CacheDir() = %q, expected to end with %q", dir, expectedSuffix)
	}
}

func TestBinaryPath(t *testing.T) {
	tests := []struct {
		name       string
		targetOS   string
		arch       string
		wantOS     string
		wantArch   string
		wantSuffix string
	}{
		{"linux_amd64", "linux", "amd64", "linux", "amd64", "amazon-gamelift-servers-game-server-wrapper"},
		{"linux_arm64", "linux", "arm64", "linux", "arm64", "amazon-gamelift-servers-game-server-wrapper"},
		{"windows_amd64", "windows", "amd64", "windows", "amd64", "amazon-gamelift-servers-game-server-wrapper.exe"},
		{"defaults", "", "", "linux", "amd64", "amazon-gamelift-servers-game-server-wrapper"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheDir := "/fake/cache"
			path := BinaryPath(cacheDir, tt.targetOS, tt.arch)

			expectedPath := filepath.Join(cacheDir, "out", tt.wantOS, tt.wantArch,
				"gamelift-servers-managed-containers", tt.wantSuffix)
			if path != expectedPath {
				t.Errorf("BinaryPath(%q, %q, %q) = %q, want %q", cacheDir, tt.targetOS, tt.arch, path, expectedPath)
			}
		})
	}
}
