package binary

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jpvelasco/ludus/internal/deploy"
)

// TestDeploy_RejectsOverlappingDirs reproduces the silent-truncation bug: an
// export whose output directory overlaps the server build directory must be
// rejected before any file is written, leaving the source intact. copyFile
// opens the source, then os.Create truncates the same inode before io.Copy
// reads it — so an overlapping export zeroes files instead of failing.
func TestDeploy_RejectsOverlappingDirs(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) (srcDir, outDir string)
		wantErr bool
	}{
		{name: "output equals source", setup: sameDirSetup, wantErr: true},
		{name: "output inside source", setup: outputInsideSourceSetup, wantErr: true},
		{name: "source inside output", setup: sourceInsideOutputSetup, wantErr: true},
		{name: "symlink alias to source", setup: symlinkAliasSetup, wantErr: true},
		{name: "sibling directories", setup: siblingSetup, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcDir, outDir := tt.setup(t)
			if err := os.WriteFile(filepath.Join(srcDir, "server.bin"), []byte("payload"), 0o644); err != nil {
				t.Fatal(err)
			}

			e := NewExporter(outDir)
			_, err := e.Deploy(context.Background(), deploy.DeployInput{ServerBuildDir: srcDir})

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Deploy() error = %v, want nil", err)
				}
				if _, err := os.Stat(filepath.Join(outDir, "server.bin")); err != nil {
					t.Errorf("normal sibling export broken: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Deploy() expected overlap rejection, got nil error")
			}
			assertSourceIntact(t, srcDir)
		})
	}
}

// assertSourceIntact verifies the export attempt did not touch the source.
func assertSourceIntact(t *testing.T, srcDir string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(srcDir, "server.bin"))
	if err != nil {
		t.Fatalf("reading source server.bin: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("source server.bin corrupted by export: %q, want %q", got, "payload")
	}
}

// sameDirSetup is the data-loss repro: the export target IS the build dir.
func sameDirSetup(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	return dir, dir
}

// outputInsideSourceSetup exports into a subdirectory of the source; the walk
// would descend into its own growing output.
func outputInsideSourceSetup(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	return dir, filepath.Join(dir, "out")
}

// sourceInsideOutputSetup copies the source contents over their own parent,
// which re-creates source-relative paths inside the source (same truncation
// hazard when names collide).
func sourceInsideOutputSetup(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "build")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	return src, root
}

// symlinkAliasSetup hides the source behind a symlink so the naive absolute
// path comparison alone would miss the overlap.
func symlinkAliasSetup(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires developer mode")
	}
	dir := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	return dir, alias
}

// siblingSetup is the normal, supported export: two disjoint directories.
func siblingSetup(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "build")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	return src, filepath.Join(root, "out")
}
