package ddc

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

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
	freed, err := Clean(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("Clean() should not error for missing dir: %v", err)
	}
	if freed != 0 {
		t.Errorf("Clean() on nonexistent dir freed = %d, want 0", freed)
	}
}

func TestClean_FileReadDirError(t *testing.T) {
	// Cleaning a path that is a regular file (not a dir) must surface an error.
	// On Windows the directory-open on a file reports ERROR_PATH_NOT_FOUND,
	// which errors.Is classifies as ErrNotExist, so Clean treats it as empty.
	file := filepath.Join(t.TempDir(), "blocker.bin")
	writeTestFile(t, file, 1)

	freed, err := Clean(file)
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("Clean(file) on windows should not error, got: %v", err)
		}
		if freed != 0 {
			t.Errorf("Clean(file) freed = %d, want 0 on windows", freed)
		}
		return
	}
	if err == nil {
		t.Fatal("Clean(file) should error on non-windows, got nil")
	}
	if !strings.Contains(err.Error(), "reading DDC directory") {
		t.Errorf("error = %v, want it to wrap the ReadDir failure", err)
	}
}

func TestClean_InvalidPathError(t *testing.T) {
	// A NUL byte in the path makes os.ReadDir fail with a non-IsNotExist
	// error on every platform, exercising the error branch deterministically.
	dir := filepath.Join(t.TempDir(), "x\x00nul")

	_, err := Clean(dir)
	if err == nil {
		t.Fatal("Clean() on invalid path should error")
	}
	if !strings.Contains(err.Error(), "reading DDC directory") {
		t.Errorf("error = %v, want it to wrap the ReadDir failure", err)
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
	freed, err := Prune(filepath.Join(t.TempDir(), "missing"), 7)
	if err != nil {
		t.Fatalf("Prune() should not error for missing dir: %v", err)
	}
	if freed != 0 {
		t.Errorf("Prune() on nonexistent dir freed = %d, want 0", freed)
	}
}

func TestPrune_NestedOldFiles(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "nested", "deep")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	oldTime := time.Now().Add(-20 * 24 * time.Hour)
	oldFile := filepath.Join(subdir, "old.bin")
	writeTestFile(t, oldFile, 512)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "fresh.bin"), 64)

	freed, err := Prune(dir, 7)
	if err != nil {
		t.Fatalf("Prune() error: %v", err)
	}
	if freed != 512 {
		t.Errorf("Prune() freed = %d, want 512 (only the nested old file)", freed)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("nested old file should have been removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "fresh.bin")); err != nil {
		t.Error("recent file should be kept")
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

func TestPruneIfOld_InfoError(t *testing.T) {
	cutoff := time.Now()
	for name, tt := range map[string]struct {
		entry       fakeDirEntry
		wantErrLike string
	}{
		"vanished": {fakeDirEntry{err: fs.ErrNotExist}, ""},
		"generic":  {fakeDirEntry{err: errors.New("stat failed")}, "stat failed"},
	} {
		t.Run(name, func(t *testing.T) {
			freed := int64(7)
			err := pruneIfOld("whatever", tt.entry, cutoff, &freed)
			if tt.wantErrLike == "" {
				if err != nil {
					t.Fatalf("pruneIfOld() = %v, want nil for vanished entry", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErrLike) {
				t.Fatalf("pruneIfOld() = %v, want error containing %q", err, tt.wantErrLike)
			}
			if freed != 7 {
				t.Errorf("freed = %d, want unchanged 7", freed)
			}
		})
	}
}

func TestPruneIfOld_RemoveError(t *testing.T) {
	cutoff := time.Now().Add(24 * time.Hour)
	info := oldTestFileInfo(t)

	for name, tt := range map[string]struct {
		path      string
		info      fs.FileInfo
		wantError bool
	}{
		"missing": {filepath.Join(t.TempDir(), "gone.bin"), info, false},
		"invalid": {filepath.Join(t.TempDir(), "\x00"), info, true},
	} {
		t.Run(name, func(t *testing.T) {
			entry := fakeDirEntry{info: tt.info}
			freed := int64(0)
			err := pruneIfOld(tt.path, entry, cutoff, &freed)
			switch {
			case tt.wantError && err == nil:
				t.Errorf("pruneIfOld() = nil, want error")
			case !tt.wantError && err != nil:
				t.Errorf("pruneIfOld() = %v, want nil", err)
			}
			if !tt.wantError && freed != 0 {
				t.Errorf("freed = %d, want 0", freed)
			}
		})
	}
}

func oldTestFileInfo(t *testing.T) fs.FileInfo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.bin")
	writeTestFile(t, path, 16)
	old := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

type fakeDirEntry struct {
	info fs.FileInfo
	err  error
}

func (f fakeDirEntry) Name() string               { return "entry" }
func (f fakeDirEntry) IsDir() bool                { return false }
func (f fakeDirEntry) Type() fs.FileMode          { return 0 }
func (f fakeDirEntry) Info() (fs.FileInfo, error) { return f.info, f.err }

func TestPrune_NonNotExistWalkError(t *testing.T) {
	// A directory path containing a NUL byte makes WalkDir fail with a
	// non-ErrNotExist error, so Prune surfaces the walk failure instead of
	// treating the path as missing.
	freed, err := Prune(filepath.Join(t.TempDir(), "\x00"), 7)
	if err == nil {
		t.Fatal("Prune() should error for a path WalkDir cannot traverse")
	}
	if freed != 0 {
		t.Errorf("Prune() freed = %d, want 0 on error", freed)
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
