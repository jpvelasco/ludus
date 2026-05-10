package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func createSDKDir(t *testing.T, engineRoot string) string {
	t.Helper()
	sdkDir := filepath.Join(engineRoot, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return sdkDir
}

func TestFindToolchainDir(t *testing.T) {
	t.Run("matching directory", func(t *testing.T) {
		dir := t.TempDir()
		tcDir := filepath.Join(dir, "v25_clang-18.1.0-rockylinux8")
		if err := os.Mkdir(tcDir, 0o755); err != nil {
			t.Fatal(err)
		}

		found, path := findToolchainDir(dir, "v25_clang-18")
		if !found {
			t.Fatal("expected to find toolchain dir")
		}
		if path != tcDir {
			t.Errorf("got path %q, want %q", path, tcDir)
		}
	})

	t.Run("no match", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "v22_clang-16.0.6-centos7"), 0o755); err != nil {
			t.Fatal(err)
		}

		found, _ := findToolchainDir(dir, "v25_clang-18")
		if found {
			t.Fatal("expected no match")
		}
	})

	t.Run("non-existent parent", func(t *testing.T) {
		found, _ := findToolchainDir("/nonexistent/path/xyz", "v25_clang-18")
		if found {
			t.Fatal("expected no match for non-existent parent")
		}
	})

	t.Run("file not dir ignored", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "v25_clang-18.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}

		found, _ := findToolchainDir(dir, "v25_clang-18")
		if found {
			t.Fatal("expected no match for file (not directory)")
		}
	})
}

func TestFindToolchainInRoot(t *testing.T) {
	t.Run("subdirectory matches", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Mkdir(filepath.Join(parent, "v26_clang-20.1.8-rockylinux8"), 0o755); err != nil {
			t.Fatal(err)
		}
		found, path := findToolchainInRoot(parent, "v26_clang-20")
		if !found {
			t.Fatal("expected match via subdirectory")
		}
		if !strings.Contains(path, "v26_clang-20") {
			t.Errorf("unexpected path: %s", path)
		}
	})

	t.Run("root dir itself matches", func(t *testing.T) {
		parent := t.TempDir()
		toolchainDir := filepath.Join(parent, "v26_clang-20.1.8-rockylinux8")
		if err := os.Mkdir(toolchainDir, 0o755); err != nil {
			t.Fatal(err)
		}
		found, path := findToolchainInRoot(toolchainDir, "v26_clang-20")
		if !found {
			t.Fatal("expected match via root dir name")
		}
		if path != toolchainDir {
			t.Errorf("got %s, want %s", path, toolchainDir)
		}
	})

	t.Run("sibling directory matches", func(t *testing.T) {
		parent := t.TempDir()
		oldDir := filepath.Join(parent, "v25_clang-18.1.0-rockylinux8")
		newDir := filepath.Join(parent, "v26_clang-20.1.8-rockylinux8")
		if err := os.Mkdir(oldDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(newDir, 0o755); err != nil {
			t.Fatal(err)
		}
		found, path := findToolchainInRoot(oldDir, "v26_clang-20")
		if !found {
			t.Fatal("expected match via sibling directory")
		}
		if !strings.Contains(path, "v26_clang-20") {
			t.Errorf("unexpected path: %s", path)
		}
	})

	t.Run("no match anywhere", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Mkdir(filepath.Join(parent, "v22_clang-16.0.6-centos7"), 0o755); err != nil {
			t.Fatal(err)
		}
		found, _ := findToolchainInRoot(parent, "v26_clang-20")
		if found {
			t.Fatal("expected no match")
		}
	})

	t.Run("root dir with trailing slash", func(t *testing.T) {
		parent := t.TempDir()
		toolchainDir := filepath.Join(parent, "v26_clang-20.1.8-rockylinux8")
		if err := os.Mkdir(toolchainDir, 0o755); err != nil {
			t.Fatal(err)
		}
		found, _ := findToolchainInRoot(toolchainDir+string(filepath.Separator), "v26_clang-20")
		if !found {
			t.Fatal("expected match with trailing slash")
		}
	})
}

func TestCheckToolchain(t *testing.T) {
	t.Run("no engine source", func(t *testing.T) {
		result := CheckToolchain("", "")
		if result.Found {
			t.Error("expected not found")
		}
		if result.Message != "skipped (no engine source path)" {
			t.Errorf("unexpected message: %s", result.Message)
		}
	})

	t.Run("version not detected", func(t *testing.T) {
		dir := t.TempDir()
		result := CheckToolchain(dir, "")
		if result.Found {
			t.Error("expected not found")
		}
		if result.Message != "could not detect engine version" {
			t.Errorf("unexpected message: %s", result.Message)
		}
	})

	t.Run("unknown version", func(t *testing.T) {
		dir := t.TempDir()
		result := CheckToolchain(dir, "5.3.0")
		if result.Found {
			t.Error("expected not found")
		}
		if result.EngineVersion != "5.3" {
			t.Errorf("got version %q, want %q", result.EngineVersion, "5.3")
		}
	})

	t.Run("toolchain found", func(t *testing.T) {
		dir := t.TempDir()
		writeBuildVersion(t, dir, 5, 6, 1)

		sdkDir := createSDKDir(t, dir)
		if err := os.Mkdir(filepath.Join(sdkDir, "v25_clang-18.1.0-rockylinux8"), 0o755); err != nil {
			t.Fatal(err)
		}

		// On Windows, CheckToolchain only checks LINUX_MULTIARCH_ROOT, not the
		// engine SDK dir. Point the env var at the SDK dir so the test works
		// on both platforms.
		if runtime.GOOS == "windows" {
			t.Setenv("LINUX_MULTIARCH_ROOT", sdkDir)
		}

		result := CheckToolchain(dir, "")
		if !result.Found {
			t.Fatalf("expected found, got message: %s", result.Message)
		}
		if result.EngineVersion != "5.6" {
			t.Errorf("got version %q, want %q", result.EngineVersion, "5.6")
		}
		if result.VersionSource != "Build.version" {
			t.Errorf("got source %q, want %q", result.VersionSource, "Build.version")
		}
	})

	t.Run("toolchain not found", func(t *testing.T) {
		dir := t.TempDir()
		writeBuildVersion(t, dir, 5, 6, 0)

		sdkDir := createSDKDir(t, dir)

		// On Windows, point LINUX_MULTIARCH_ROOT at the empty SDK dir so the
		// code path that checks for the toolchain is exercised.
		if runtime.GOOS == "windows" {
			t.Setenv("LINUX_MULTIARCH_ROOT", sdkDir)
		}

		result := CheckToolchain(dir, "")
		if result.Found {
			t.Error("expected not found")
		}
		if result.EngineVersion != "5.6" {
			t.Errorf("got version %q, want %q", result.EngineVersion, "5.6")
		}
	})
}
