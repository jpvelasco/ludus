package wrapper

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
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

// setupTestHome redirects os.UserHomeDir() to a temp directory and returns
// the resulting CacheDir path. Used by EnsureBinary tests to isolate the
// cache without touching the real home directory.
func setupTestHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmpDir)
	} else {
		t.Setenv("HOME", tmpDir)
	}
	cacheDir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error: %v", err)
	}
	return cacheDir
}

// seedFakeBinary creates a fake cached binary at the expected BinaryPath location.
func seedFakeBinary(t *testing.T, cacheDir, targetOS, arch string) string {
	t.Helper()
	binaryPath := BinaryPath(cacheDir, targetOS, arch)
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("fake-binary"), 0755); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	return binaryPath
}

// TestEnsureBinaryCacheHit verifies that EnsureBinary returns the cached binary
// path without cloning or building when the binary already exists on disk.
// The runner is unconfigured, so any attempt to clone/build would fail —
// proving the cache-hit path short-circuits correctly.
func TestEnsureBinaryCacheHit(t *testing.T) {
	tests := []struct {
		name     string
		targetOS string
		arch     string
	}{
		{"linux_amd64", "linux", "amd64"},
		{"linux_arm64", "linux", "arm64"},
		{"windows_amd64", "windows", "amd64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheDir := setupTestHome(t)
			binaryPath := seedFakeBinary(t, cacheDir, tt.targetOS, tt.arch)

			r := &runner.Runner{}
			got, err := EnsureBinary(context.Background(), r, tt.targetOS, tt.arch)
			if err != nil {
				t.Fatalf("EnsureBinary() error: %v", err)
			}
			if got != binaryPath {
				t.Errorf("EnsureBinary() = %q, want %q", got, binaryPath)
			}
		})
	}
}

// TestEnsureBinaryDefaults verifies that empty targetOS/arch default to linux/amd64.
func TestEnsureBinaryDefaults(t *testing.T) {
	cacheDir := setupTestHome(t)
	binaryPath := seedFakeBinary(t, cacheDir, "linux", "amd64")

	r := &runner.Runner{}
	got, err := EnsureBinary(context.Background(), r, "", "")
	if err != nil {
		t.Fatalf("EnsureBinary() error: %v", err)
	}
	if got != binaryPath {
		t.Errorf("EnsureBinary(\"\", \"\") = %q, want %q (linux/amd64 default)", got, binaryPath)
	}
}
