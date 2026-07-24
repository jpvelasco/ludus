//go:build windows

package dockerbuild

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name+".exe")
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}
