package toolchain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeBuildVersion(t *testing.T, engineRoot string, major, minor, patch int) {
	t.Helper()
	versionDir := filepath.Join(engineRoot, "Engine", "Build")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(BuildVersion{MajorVersion: major, MinorVersion: minor, PatchVersion: patch})
	if err := os.WriteFile(filepath.Join(versionDir, "Build.version"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseBuildVersion(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		dir := t.TempDir()
		writeBuildVersion(t, dir, 5, 6, 1)

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
		writeBuildVersion(t, dir, 0, 0, 0)
		if err := os.WriteFile(filepath.Join(dir, "Engine", "Build", "Build.version"), []byte("{bad json"), 0o644); err != nil {
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
		writeBuildVersion(t, dir, 5, 7, 0)

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
