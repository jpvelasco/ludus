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

func TestBinaryPath_Amd64(t *testing.T) {
	cacheDir := "/fake/cache"
	path := BinaryPath(cacheDir, "amd64")

	if !strings.Contains(path, "amd64") {
		t.Errorf("BinaryPath with amd64 should contain 'amd64', got %q", path)
	}

	expectedBinaryName := "amazon-gamelift-servers-game-server-wrapper"
	if !strings.HasSuffix(path, expectedBinaryName) {
		t.Errorf("BinaryPath should end with %q, got %q", expectedBinaryName, path)
	}

	// Verify full expected structure
	expectedPath := filepath.Join(cacheDir, "out", "linux", "amd64",
		"gamelift-servers-managed-containers", "amazon-gamelift-servers-game-server-wrapper")
	if path != expectedPath {
		t.Errorf("BinaryPath(cacheDir, 'amd64') = %q, want %q", path, expectedPath)
	}
}

func TestBinaryPath_Arm64(t *testing.T) {
	cacheDir := "/fake/cache"
	path := BinaryPath(cacheDir, "arm64")

	if !strings.Contains(path, "arm64") {
		t.Errorf("BinaryPath with arm64 should contain 'arm64', got %q", path)
	}

	expectedBinaryName := "amazon-gamelift-servers-game-server-wrapper"
	if !strings.HasSuffix(path, expectedBinaryName) {
		t.Errorf("BinaryPath should end with %q, got %q", expectedBinaryName, path)
	}

	// Verify full expected structure
	expectedPath := filepath.Join(cacheDir, "out", "linux", "arm64",
		"gamelift-servers-managed-containers", "amazon-gamelift-servers-game-server-wrapper")
	if path != expectedPath {
		t.Errorf("BinaryPath(cacheDir, 'arm64') = %q, want %q", path, expectedPath)
	}
}

func TestBinaryPath_EmptyArch(t *testing.T) {
	cacheDir := "/fake/cache"
	path := BinaryPath(cacheDir, "")

	// Empty arch should default to amd64
	if !strings.Contains(path, "amd64") {
		t.Errorf("BinaryPath with empty arch should default to 'amd64', got %q", path)
	}

	expectedPath := filepath.Join(cacheDir, "out", "linux", "amd64",
		"gamelift-servers-managed-containers", "amazon-gamelift-servers-game-server-wrapper")
	if path != expectedPath {
		t.Errorf("BinaryPath(cacheDir, '') = %q, want %q (should default to amd64)", path, expectedPath)
	}
}
