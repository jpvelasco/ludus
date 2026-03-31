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
		name     string
		arch     string
		wantArch string
	}{
		{"amd64", "amd64", "amd64"},
		{"arm64", "arm64", "arm64"},
		{"empty", "", "amd64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheDir := "/fake/cache"
			path := BinaryPath(cacheDir, tt.arch)

			if !strings.Contains(path, tt.wantArch) {
				t.Errorf("BinaryPath(%q, %q) should contain %q, got %q", cacheDir, tt.arch, tt.wantArch, path)
			}

			binaryName := "amazon-gamelift-servers-game-server-wrapper"
			if !strings.HasSuffix(path, binaryName) {
				t.Errorf("BinaryPath(%q, %q) should end with %q, got %q", cacheDir, tt.arch, binaryName, path)
			}

			expectedPath := filepath.Join(cacheDir, "out", "linux", tt.wantArch,
				"gamelift-servers-managed-containers", "amazon-gamelift-servers-game-server-wrapper")
			if path != expectedPath {
				t.Errorf("BinaryPath(%q, %q) = %q, want %q", cacheDir, tt.arch, path, expectedPath)
			}
		})
	}
}
