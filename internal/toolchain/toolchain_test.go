package toolchain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseBuildVersion(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		dir := t.TempDir()
		versionDir := filepath.Join(dir, "Engine", "Build")
		if err := os.MkdirAll(versionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(BuildVersion{MajorVersion: 5, MinorVersion: 6, PatchVersion: 1})
		if err := os.WriteFile(filepath.Join(versionDir, "Build.version"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		v, err := ParseBuildVersion(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.MajorVersion != 5 || v.MinorVersion != 6 || v.PatchVersion != 1 {
			t.Errorf("got %+v, want 5.6.1", v)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		dir := t.TempDir()
		_, err := ParseBuildVersion(dir)
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		versionDir := filepath.Join(dir, "Engine", "Build")
		if err := os.MkdirAll(versionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(versionDir, "Build.version"), []byte("{bad json"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := ParseBuildVersion(dir)
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})
}

func TestDetectEngineVersion(t *testing.T) {
	t.Run("Build.version present", func(t *testing.T) {
		dir := t.TempDir()
		versionDir := filepath.Join(dir, "Engine", "Build")
		if err := os.MkdirAll(versionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(BuildVersion{MajorVersion: 5, MinorVersion: 7, PatchVersion: 0})
		if err := os.WriteFile(filepath.Join(versionDir, "Build.version"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		version, source := DetectEngineVersion(dir, "5.6.1")
		if version != "5.7" {
			t.Errorf("got version %q, want %q", version, "5.7")
		}
		if source != "Build.version" {
			t.Errorf("got source %q, want %q", source, "Build.version")
		}
	})

	t.Run("missing Build.version falls back to config", func(t *testing.T) {
		dir := t.TempDir()
		version, source := DetectEngineVersion(dir, "5.6.1")
		if version != "5.6" {
			t.Errorf("got version %q, want %q", version, "5.6")
		}
		if source != "config" {
			t.Errorf("got source %q, want %q", source, "config")
		}
	})

	t.Run("both missing", func(t *testing.T) {
		dir := t.TempDir()
		version, source := DetectEngineVersion(dir, "")
		if version != "" {
			t.Errorf("got version %q, want empty", version)
		}
		if source != "" {
			t.Errorf("got source %q, want empty", source)
		}
	})

	t.Run("empty engine path with config", func(t *testing.T) {
		version, source := DetectEngineVersion("", "5.5.0")
		if version != "5.5" {
			t.Errorf("got version %q, want %q", version, "5.5")
		}
		if source != "config" {
			t.Errorf("got source %q, want %q", source, "config")
		}
	})

	t.Run("config with only major version", func(t *testing.T) {
		version, source := DetectEngineVersion("", "5")
		if version != "" {
			t.Errorf("got version %q, want empty (single component)", version)
		}
		if source != "" {
			t.Errorf("got source %q, want empty", source)
		}
	})
}

func TestLookupToolchain(t *testing.T) {
	tests := []struct {
		version string
		wantNil bool
		clang   int
	}{
		{"5.4", false, 16},
		{"5.5", false, 18},
		{"5.6", false, 18},
		{"5.7", false, 20},
		{"5.3", true, 0},
		{"", true, 0},
		{"6.0", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			spec := LookupToolchain(tt.version)
			if tt.wantNil {
				if spec != nil {
					t.Errorf("expected nil for version %q, got %+v", tt.version, spec)
				}
				return
			}
			if spec == nil {
				t.Fatalf("expected spec for version %q, got nil", tt.version)
			}
			if spec.ClangMajor != tt.clang {
				t.Errorf("got ClangMajor %d, want %d", spec.ClangMajor, tt.clang)
			}
		})
	}
}

func TestFindToolchainDir(t *testing.T) {
	t.Run("matching directory", func(t *testing.T) {
		dir := t.TempDir()
		tcDir := filepath.Join(dir, "v22_clang-18.1.8-centos7")
		if err := os.Mkdir(tcDir, 0o755); err != nil {
			t.Fatal(err)
		}

		found, path := findToolchainDir(dir, "v22_clang-18")
		if !found {
			t.Fatal("expected to find toolchain dir")
		}
		if path != tcDir {
			t.Errorf("got path %q, want %q", path, tcDir)
		}
	})

	t.Run("no match", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "v21_clang-16.0.6-centos7"), 0o755); err != nil {
			t.Fatal(err)
		}

		found, _ := findToolchainDir(dir, "v22_clang-18")
		if found {
			t.Fatal("expected no match")
		}
	})

	t.Run("non-existent parent", func(t *testing.T) {
		found, _ := findToolchainDir("/nonexistent/path/xyz", "v22_clang-18")
		if found {
			t.Fatal("expected no match for non-existent parent")
		}
	})

	t.Run("file not dir ignored", func(t *testing.T) {
		dir := t.TempDir()
		// Create a file (not dir) with matching prefix
		if err := os.WriteFile(filepath.Join(dir, "v22_clang-18.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}

		found, _ := findToolchainDir(dir, "v22_clang-18")
		if found {
			t.Fatal("expected no match for file (not directory)")
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

		// Create Build.version
		versionDir := filepath.Join(dir, "Engine", "Build")
		if err := os.MkdirAll(versionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(BuildVersion{MajorVersion: 5, MinorVersion: 6, PatchVersion: 1})
		if err := os.WriteFile(filepath.Join(versionDir, "Build.version"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		// Create toolchain dir in the engine SDK location
		sdkDir := filepath.Join(dir, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64")
		if err := os.MkdirAll(sdkDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(sdkDir, "v22_clang-18.1.8-centos7"), 0o755); err != nil {
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

		// Create Build.version for 5.6
		versionDir := filepath.Join(dir, "Engine", "Build")
		if err := os.MkdirAll(versionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(BuildVersion{MajorVersion: 5, MinorVersion: 6, PatchVersion: 0})
		if err := os.WriteFile(filepath.Join(versionDir, "Build.version"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		// Create the SDK dir but without matching toolchain
		sdkDir := filepath.Join(dir, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64")
		if err := os.MkdirAll(sdkDir, 0o755); err != nil {
			t.Fatal(err)
		}

		// On Windows, point LINUX_MULTIARCH_ROOT at the empty SDK dir so the
		// code path that checks for the toolchain is exercised (rather than
		// returning the "env var not set" message).
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
