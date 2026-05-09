package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashDeterministic(t *testing.T) {
	h1 := hash("a", "b", "c")
	h2 := hash("a", "b", "c")
	if h1 != h2 {
		t.Fatal("hash not deterministic")
	}
	if h1 == hash("a", "b", "d") {
		t.Fatal("different inputs should produce different hashes")
	}
}

func TestDirManifest(t *testing.T) {
	tmpDir := t.TempDir()

	writeFile := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("a.txt", "hello")
	writeFile("b.txt", "world!")
	if err := os.MkdirAll(filepath.Join(tmpDir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(filepath.Join("sub", "c.txt"), "x")

	m := dirManifest(tmpDir)
	if m == "" {
		t.Fatal("expected non-empty manifest")
	}
	if m != dirManifest(tmpDir) {
		t.Fatal("manifest not deterministic")
	}
}

func TestGitHEAD_NoRepo(t *testing.T) {
	if head := gitHEAD(t.TempDir()); head != "" {
		t.Errorf("gitHEAD on non-repo should return empty string, got %q", head)
	}
}

func TestFileKey(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		key := fileKey(filepath.Join(t.TempDir(), "does-not-exist.txt"))
		if key != "" {
			t.Errorf("expected empty string for missing file, got %q", key)
		}
	})

	t.Run("existing file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "test.txt")
		if err := os.WriteFile(f, []byte("hello world"), 0644); err != nil {
			t.Fatal(err)
		}

		key := fileKey(f)
		if key == "" {
			t.Fatal("expected non-empty key for existing file")
		}
		if !strings.Contains(key, ":") {
			t.Errorf("expected mtime:size format, got %q", key)
		}
		// "hello world" is 11 bytes
		if suffix := fmt.Sprintf(":%d", 11); !strings.HasSuffix(key, suffix) {
			t.Errorf("expected key to end with %q, got %q", suffix, key)
		}
	})
}
