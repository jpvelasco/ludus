package wrapper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

func TestUsesMake(t *testing.T) {
	// UsesMake is true only for a native linux/amd64 build on a Linux host.
	// On a non-Linux host it must be false for every target/arch (those builds
	// cross-compile with `go build` and never shell out to make).
	tests := []struct {
		targetOS, arch string
		wantOnLinux    bool
	}{
		{"linux", "amd64", true},
		{"linux", "arm64", false},
		{"windows", "amd64", false},
		{"", "", false},
	}

	hostLinux := runtime.GOOS == "linux"
	for _, tt := range tests {
		want := tt.wantOnLinux && hostLinux
		if got := UsesMake(tt.targetOS, tt.arch); got != want {
			t.Errorf("UsesMake(%q, %q) on %s = %v, want %v",
				tt.targetOS, tt.arch, runtime.GOOS, got, want)
		}
	}
}

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
func TestIsBinaryCached(t *testing.T) {
	cacheDir := setupTestHome(t)

	if IsBinaryCached("linux", "arm64") {
		t.Fatal("IsBinaryCached() = true before binary exists")
	}
	seedFakeBinary(t, cacheDir, "linux", "arm64")
	if !IsBinaryCached("linux", "arm64") {
		t.Fatal("IsBinaryCached() = false after binary was cached")
	}
	if IsBinaryCached("windows", "arm64") {
		t.Fatal("IsBinaryCached() matched a different target OS")
	}
}

func TestVerifyServerSDKChecksumMatch(t *testing.T) {
	archive, expected := writeSDKTestArchive(t, []byte("test server SDK archive"))
	if err := verifyServerSDKChecksum(archive, expected); err != nil {
		t.Fatalf("verifyServerSDKChecksum() error = %v, want match", err)
	}
}

func TestVerifyServerSDKChecksumMismatchRemovesArchive(t *testing.T) {
	archive, _ := writeSDKTestArchive(t, []byte("test server SDK archive"))
	err := verifyServerSDKChecksum(archive, strings.Repeat("0", sha256.Size*2))
	if err == nil || !strings.Contains(err.Error(), "server SDK checksum mismatch") {
		t.Fatalf("verifyServerSDKChecksum() error = %v, want checksum mismatch", err)
	}
	if _, statErr := os.Stat(archive); !os.IsNotExist(statErr) {
		t.Errorf("archive stat error = %v, want archive removed", statErr)
	}
}

func writeSDKTestArchive(t *testing.T, data []byte) (string, string) {
	t.Helper()
	archive := filepath.Join(t.TempDir(), "sdk.zip")
	if err := os.WriteFile(archive, data, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return archive, hex.EncodeToString(digest[:])
}

func TestDownloadWrapperSourceRejectsCachedSDKChecksumMismatch(t *testing.T) {
	cacheDir := t.TempDir()
	archive := filepath.Join(cacheDir, "gamelift-servers-server-sdk.zip")
	if err := os.WriteFile(archive, []byte("unverified cached SDK"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := downloadWrapperSource(context.Background(), &runner.Runner{DryRun: true}, cacheDir)
	if err == nil || !strings.Contains(err.Error(), "server SDK checksum mismatch") {
		t.Fatalf("downloadWrapperSource() error = %v, want cached SDK checksum mismatch", err)
	}
	if _, statErr := os.Stat(archive); !os.IsNotExist(statErr) {
		t.Errorf("cached archive stat error = %v, want archive removed", statErr)
	}
	sdkDir := filepath.Join(cacheDir, "src", "ext", "gamelift-servers-server-sdk")
	if _, statErr := os.Stat(sdkDir); !os.IsNotExist(statErr) {
		t.Errorf("SDK directory stat error = %v, want extraction skipped", statErr)
	}
}
