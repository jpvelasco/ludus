package container

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

func TestCopyFile(t *testing.T) {
	t.Run("copies content and preserves mode", testCopyFileSuccess)
	t.Run("missing source errors", testCopyFileMissingSource)
	t.Run("unwritable destination errors", testCopyFileBadDest)
}

func testCopyFileSuccess(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	dst := filepath.Join(dir, "dst.bin")
	if err := os.WriteFile(src, []byte("hello wrapper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile error: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello wrapper" {
		t.Errorf("content = %q, want %q", got, "hello wrapper")
	}
	// Mode preservation is meaningful on Unix; Windows reports 0666/0777.
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("dst mode = %v, want 0755", info.Mode().Perm())
	}
}

func testCopyFileMissingSource(t *testing.T) {
	dir := t.TempDir()
	if err := copyFile(filepath.Join(dir, "nope.bin"), filepath.Join(dir, "out.bin")); err == nil {
		t.Error("expected error for missing source")
	}
}

func testCopyFileBadDest(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A destination path whose parent is a file (not a dir) cannot be created.
	badDst := filepath.Join(src, "child.bin")
	if err := copyFile(src, badDst); err == nil {
		t.Error("expected error when destination cannot be created")
	}
}

func TestGenerateAndWriteDockerfile_DryRunIsNoop(t *testing.T) {
	dir := t.TempDir()
	b := NewBuilder(BuildOptions{
		ServerBuildDir: dir,
		ImageName:      "ludus-server",
		Tag:            "latest",
		ServerPort:     7777,
		ProjectName:    "Lyra",
	}, runner.NewRunner(false, true)) // dry-run

	cleanup, err := b.generateAndWriteDockerfile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cleanup() // must be safe to call

	// In dry-run, nothing should be staged into the build dir.
	for _, f := range []string{"Dockerfile", ".dockerignore", "config.yaml",
		"amazon-gamelift-servers-game-server-wrapper"} {
		if _, statErr := os.Stat(filepath.Join(dir, f)); statErr == nil {
			t.Errorf("dry-run should not have written %s", f)
		}
	}
}
