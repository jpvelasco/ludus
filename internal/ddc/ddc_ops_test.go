package ddc

import (
	"os"
	"path/filepath"
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
