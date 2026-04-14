package ddc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidateDDCMode(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"", "local", false},
		{"local", "local", false},
		{"none", "none", false},
		{"shared", "", true},
		{"LOCAL", "", true},
	}
	for _, tt := range tests {
		got, err := ValidateDDCMode(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateDDCMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ValidateDDCMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDefaultPath(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error: %v", err)
	}

	want := filepath.Join(home, ".ludus", "ddc")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestResolvePath_Override(t *testing.T) {
	path := "/custom/ddc"
	if runtime.GOOS == "windows" {
		path = `C:\custom\ddc`
	}
	got, err := ResolvePath(path)
	if err != nil {
		t.Fatalf("ResolvePath() error: %v", err)
	}
	if got != path {
		t.Errorf("ResolvePath(%q) = %q, want %q", path, got, path)
	}
}

func TestResolvePath_RelativeErrors(t *testing.T) {
	_, err := ResolvePath("relative/ddc")
	if err == nil {
		t.Error("ResolvePath() should error for relative path")
	}
}

func TestResolvePath_Default(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}

	got, err := ResolvePath("")
	if err != nil {
		t.Fatalf("ResolvePath() error: %v", err)
	}

	want := filepath.Join(home, ".ludus", "ddc")
	if got != want {
		t.Errorf("ResolvePath(%q) = %q, want %q", "", got, want)
	}
}

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
	if err := os.MkdirAll(sub, 0755); err != nil {
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

func TestClean(t *testing.T) {
	dir := t.TempDir()
	populateTestDir(t, dir)

	freed, err := Clean(dir)
	if err != nil {
		t.Fatalf("Clean() error: %v", err)
	}
	assertBytesFreed(t, freed, 3584)
	assertDirEmpty(t, dir)
}

func TestClean_Empty(t *testing.T) {
	dir := t.TempDir()

	freed, err := Clean(dir)
	if err != nil {
		t.Fatalf("Clean() error: %v", err)
	}
	if freed != 0 {
		t.Errorf("Clean() on empty dir freed = %d, want 0", freed)
	}
}

func TestClean_NotExist(t *testing.T) {
	freed, err := Clean("/nonexistent/path")
	if err != nil {
		t.Fatalf("Clean() should not error for missing dir: %v", err)
	}
	if freed != 0 {
		t.Errorf("Clean() on nonexistent dir freed = %d, want 0", freed)
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()

	now := time.Now()
	oldTime := now.Add(-10 * 24 * time.Hour)

	oldFile := filepath.Join(dir, "old.bin")
	writeTestFile(t, oldFile, 1024)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	newFile := filepath.Join(dir, "new.bin")
	writeTestFile(t, newFile, 2048)

	freed, err := Prune(dir, 7)
	if err != nil {
		t.Fatalf("Prune() error: %v", err)
	}
	if freed != 1024 {
		t.Errorf("Prune() freed = %d, want 1024", freed)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("old file still exists")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Errorf("new file should exist: %v", err)
	}
}

func TestPrune_NotExist(t *testing.T) {
	freed, err := Prune("/nonexistent/path", 7)
	if err != nil {
		t.Fatalf("Prune() should not error for missing dir: %v", err)
	}
	if freed != 0 {
		t.Errorf("Prune() on nonexistent dir freed = %d, want 0", freed)
	}
}

func TestPrune_InvalidDays(t *testing.T) {
	tests := []struct {
		name string
		days int
	}{
		{"zero", 0},
		{"negative", -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Prune(t.TempDir(), tt.days)
			if err == nil {
				t.Errorf("Prune(days=%d) should error", tt.days)
			}
			if !strings.Contains(err.Error(), "at least 1 day") {
				t.Errorf("error should mention minimum, got: %v", err)
			}
		})
	}
}

func TestEnvOverride(t *testing.T) {
	got := EnvOverride("/ddc")
	want := "UE-LocalDataCachePath=/ddc"
	if got != want {
		t.Errorf("EnvOverride(/ddc) = %q, want %q", got, want)
	}
}

func TestEnvOverride_WindowsPath(t *testing.T) {
	got := EnvOverride(`C:\Users\test\.ludus\ddc`)
	want := "UE-LocalDataCachePath=C:/Users/test/.ludus/ddc"
	if got != want {
		t.Errorf("EnvOverride() = %q, want %q", got, want)
	}
}

// populateTestDir creates a test directory with files totaling 3584 bytes.
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
