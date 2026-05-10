package ddc

import (
	"os"
	"path/filepath"
	"testing"
)

func populateTestDir(t *testing.T, dir string) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, "file1.bin"), 1024)
	writeTestFile(t, filepath.Join(dir, "file2.bin"), 2048)
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(subdir, "file3.bin"), 512)
}

func writeTestFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0644); err != nil {
		t.Fatal(err)
	}
}

func mkdirAll(t *testing.T, path string) error {
	t.Helper()
	return os.MkdirAll(path, 0755)
}

func assertBytesFreed(t *testing.T, got, want int64) {
	t.Helper()
	if got != want {
		t.Errorf("bytes freed = %d, want %d", got, want)
	}
}

func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("dir has %d entries, want 0", len(entries))
	}
}
