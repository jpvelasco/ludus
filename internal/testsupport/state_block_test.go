package testsupport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/internal/state"
)

// TestBlockStateWrite verifies that BlockStateWrite changes into a fresh dir and
// makes state.Save fail, so callers of the helper actually exercise their
// state-write error branches.
func TestBlockStateWrite(t *testing.T) {
	BlockStateWrite(t)

	if _, err := os.Stat(".ludus"); err != nil {
		t.Fatalf("expected .ludus to exist after BlockStateWrite: %v", err)
	}
	if info, err := os.Stat(filepath.Join(".ludus", "state.json")); err == nil || info != nil {
		t.Fatal("expected .ludus/state.json to be unreadable when .ludus is a file")
	}

	if err := state.Save(&state.State{}); err == nil {
		t.Fatal("expected state.Save to fail after BlockStateWrite")
	}
}
