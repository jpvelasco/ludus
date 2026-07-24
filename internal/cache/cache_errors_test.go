package cache

import (
	"os"
	"testing"
)

// TestSave_MkdirAllFails covers the error branch when .ludus cannot be created
// because a file already occupies that path.
func TestSave_MkdirAllFails(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.WriteFile(cacheDir, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	c := &Cache{Entries: make(map[StageKey]*Entry)}
	if err := Save(c); err == nil {
		t.Fatal("expected error when cacheDir path is occupied by a file")
	}
}

// TestRecordBuild_LoadFailsIsNoop verifies that RecordBuild silently no-ops
// (does not panic, does not write) when the existing cache.json is corrupted.
func TestRecordBuild_LoadFailsIsNoop(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath(), []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}

	RecordBuild(StageEngine, "hash-1", false)

	data, err := os.ReadFile(cachePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{not valid json" {
		t.Errorf("RecordBuild should not have modified the corrupted cache file, got: %s", data)
	}
}

// TestCheckSkip_LoadFailsReturnsFalse verifies CheckSkip returns false
// (proceed with build) rather than erroring when cache.json is corrupted.
func TestCheckSkip_LoadFailsReturnsFalse(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath(), []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}

	if got := CheckSkip(StageEngine, "any-hash", "TestProject", false); got {
		t.Error("expected CheckSkip to return false when cache load fails")
	}
}
