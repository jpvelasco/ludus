//go:build linux

package prereq

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverlayLyraContent_OnLinux(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "Content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "Content", "DefaultGameData.uasset"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "Lyra")

	res := (&Checker{}).overlayLyraContent(src, dst)
	if !res.Passed || !strings.Contains(res.Message, "overlaid from") {
		t.Fatalf("overlayLyraContent() = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dst, "Content", "DefaultGameData.uasset")); err != nil {
		t.Fatalf("overlay did not copy content: %v", err)
	}
}
