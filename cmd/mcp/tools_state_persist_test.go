package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveWSL2EngineResultSurfacesPersistError pins the #543 contract: state
// persistence failures must be returned to the caller (which appends them to
// the tool result) instead of being discarded while the result reports
// success. .ludus/state.json is replaced by a directory so every state write
// fails deterministically.
func TestSaveWSL2EngineResultSurfacesPersistError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join(".ludus", "state.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := saveWSL2EngineResult("/mnt/e/engine", "", "hash", false, true)
	if err == nil {
		t.Fatal("saveWSL2EngineResult() error = nil, want persistence failure")
	}

	warn := persistFailedWarn(err)
	if !strings.Contains(warn, ".ludus state could not be updated") {
		t.Errorf("persistFailedWarn() = %q, want state-persistence warning", warn)
	}
}
