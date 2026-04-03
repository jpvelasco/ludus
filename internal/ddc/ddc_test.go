package ddc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

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
	got, err := ResolvePath("/custom/ddc")
	if err != nil {
		t.Fatalf("ResolvePath() error: %v", err)
	}
	if got != "/custom/ddc" {
		t.Errorf("ResolvePath(%q) = %q, want %q", "/custom/ddc", got, "/custom/ddc")
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

func TestDirSize(t *testing.T) {
	dir := t.TempDir()

	// Empty directory
	size, err := DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize() error: %v", err)
	}
	if size != 0 {
		t.Errorf("empty dir size = %d, want 0", size)
	}

	// Add a file
	if err := os.WriteFile(filepath.Join(dir, "test.bin"), make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}

	size, err = DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize() error: %v", err)
	}
	if size != 1024 {
		t.Errorf("dir size = %d, want 1024", size)
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

	// Create some files
	file1 := filepath.Join(dir, "file1.bin")
	file2 := filepath.Join(dir, "file2.bin")
	if err := os.WriteFile(file1, make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, make([]byte, 2048), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory with a file
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	file3 := filepath.Join(subdir, "file3.bin")
	if err := os.WriteFile(file3, make([]byte, 512), 0644); err != nil {
		t.Fatal(err)
	}

	// Clean should return total bytes
	freed, err := Clean(dir)
	if err != nil {
		t.Fatalf("Clean() error: %v", err)
	}
	if freed != 3584 {
		t.Errorf("Clean() freed = %d, want 3584", freed)
	}

	// Verify files are gone
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("Clean() left %d entries, want 0", len(entries))
	}
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

	// Create old file
	oldFile := filepath.Join(dir, "old.bin")
	if err := os.WriteFile(oldFile, make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// Create new file
	newFile := filepath.Join(dir, "new.bin")
	if err := os.WriteFile(newFile, make([]byte, 2048), 0644); err != nil {
		t.Fatal(err)
	}

	// Prune files older than 7 days
	freed, err := Prune(dir, 7)
	if err != nil {
		t.Fatalf("Prune() error: %v", err)
	}
	if freed != 1024 {
		t.Errorf("Prune() freed = %d, want 1024", freed)
	}

	// Verify old file is gone, new file remains
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

func TestIniSection(t *testing.T) {
	section := IniSection("/ddc")
	if !strings.Contains(section, "[DerivedDataBackendGraph]") {
		t.Error("IniSection missing section header")
	}
	if !strings.Contains(section, "Default=Async") {
		t.Error("IniSection missing Default=Async")
	}
	if !strings.Contains(section, `Root="/ddc"`) {
		t.Error("IniSection missing Root path")
	}
}

func TestIniSection_WindowsPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("filepath.ToSlash is a no-op on non-Windows")
	}
	section := IniSection(`C:\Users\test\.ludus\ddc`)
	// UE5 ini files use forward slashes
	if !strings.Contains(section, `Root="C:/Users/test/.ludus/ddc"`) {
		t.Errorf("IniSection should convert backslashes to forward slashes, got: %s", section)
	}
}

func TestPatchProjectINI(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "Config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	iniPath := filepath.Join(configDir, "DefaultEngine.ini")
	original := "[/Script/Engine.GameEngine]\nSomeSetting=true\n"
	if err := os.WriteFile(iniPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	restore, err := PatchProjectINI(iniPath, "/ddc")
	if err != nil {
		t.Fatalf("PatchProjectINI() error: %v", err)
	}

	// Verify DDC section was added
	data, err := os.ReadFile(iniPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "DerivedDataBackendGraph") {
		t.Error("PatchProjectINI did not add DDC section")
	}

	// Verify restore reverts
	restore()
	data, err = os.ReadFile(iniPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("restore did not revert file: got %q, want %q", string(data), original)
	}
}

func TestPatchProjectINI_AlreadyPatched(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "Config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	iniPath := filepath.Join(configDir, "DefaultEngine.ini")
	content := "[DerivedDataBackendGraph]\nDefault=Shared\n"
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	restore, err := PatchProjectINI(iniPath, "/ddc")
	if err != nil {
		t.Fatalf("PatchProjectINI() error: %v", err)
	}
	restore()

	// File should be unchanged
	data, err := os.ReadFile(iniPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("already-patched file was modified: got %q", string(data))
	}
}

func TestPatchProjectINI_NotExist(t *testing.T) {
	restore, err := PatchProjectINI("/nonexistent/DefaultEngine.ini", "/ddc")
	if err != nil {
		t.Fatalf("PatchProjectINI() should not error for missing file: %v", err)
	}
	restore() // should be a no-op
}
