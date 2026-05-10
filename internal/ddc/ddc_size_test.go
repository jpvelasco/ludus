package ddc

import (
	"path/filepath"
	"testing"
)

// TestDirSize_EmptyPath guards against DirSize("") walking cwd on Linux/macOS.
// When path is empty (e.g. mode=none), DirSize must return 0 without error.
func TestDirSize_EmptyPath(t *testing.T) {
	size, err := DirSize("")
	if err != nil {
		t.Fatalf("DirSize(\"\") error = %v, want nil", err)
	}
	if size != 0 {
		t.Errorf("DirSize(\"\") = %d, want 0", size)
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()

	size, err := DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize() error: %v", err)
	}
	if size != 0 {
		t.Errorf("empty dir size = %d, want 0", size)
	}

	writeTestFile(t, filepath.Join(dir, "test.bin"), 1024)

	size, err = DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize() error: %v", err)
	}
	if size != 1024 {
		t.Errorf("dir size = %d, want 1024", size)
	}
}

func TestDirSize_Nested(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	if err := mkdirAll(t, sub); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "top.bin"), 100)
	writeTestFile(t, filepath.Join(sub, "deep.bin"), 200)

	size, err := DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize() error: %v", err)
	}
	if size != 300 {
		t.Errorf("nested dir size = %d, want 300", size)
	}
}

func TestDirSize_NotExist(t *testing.T) {
	size, err := DirSize("/nonexistent/path")
	if err != nil {
		t.Fatalf("DirSize() should not error for missing dir: %v", err)
	}
	if size != 0 {
		t.Errorf("nonexistent dir size = %d, want 0", size)
	}
}
