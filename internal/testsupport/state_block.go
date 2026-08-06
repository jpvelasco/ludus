package testsupport

import (
	"os"
	"testing"
)

// BlockStateWrite changes into a fresh temp dir and places a regular file
// where the .ludus state directory should be, so state.Save/Update calls fail
// with a non-IsNotExist error. Use it to drive warning/error branches that
// depend on a state-write failure.
func BlockStateWrite(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".ludus", []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
}
