package container

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/wrapper"
)

// stagedFileNames are the paths container staging writes into the packaged
// server directory — exactly the files a user may already own there (#734).
var stagedFileNames = []string{
	"amazon-gamelift-servers-game-server-wrapper",
	"config.yaml",
	"Dockerfile",
	".dockerignore",
}

// writeUserFile creates dir/name with sentinel content owned by the "user".
func writeUserFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("user-owned "+name), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertUserFilesRestored verifies the named user files survived staging with
// their original content and that no backup directory is left behind.
func assertUserFilesRestored(t *testing.T, dir string, names []string) {
	t.Helper()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("user file %s was lost by container staging: %v", name, err)
			continue
		}
		if want := "user-owned " + name; string(data) != want {
			t.Errorf("user file %s content = %q, want %q", name, string(data), want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, stagingBackupDirName)); err == nil {
		t.Errorf("staging backup directory %s should be removed after cleanup", stagingBackupDirName)
	}
}

// newStagingBuilder returns a builder whose wrapper resolves to the cached
// stub, plus the packaged directory it stages into.
func newStagingBuilder(t *testing.T) (*Builder, string) {
	t.Helper()
	stubCachedWrapper(t, "amd64")
	dir := t.TempDir()
	b := NewBuilder(BuildOptions{
		ServerBuildDir:  dir,
		ImageName:       "ludus-server",
		Tag:             "latest",
		ServerPort:      7777,
		ProjectName:     "MyGame",
		ServerTarget:    "MyGameServer",
		PackagedDirName: "MyGame",
	}, runner.NewRunner(false, false))
	return b, dir
}

// TestGenerateAndWriteDockerfile_PreservesExistingFiles pins the #734 fix:
// staging temporarily replaces pre-existing files in the packaged server
// directory, and the cleanup restores the user's originals instead of
// deleting them.
func TestGenerateAndWriteDockerfile_PreservesExistingFiles(t *testing.T) {
	b, dir := newStagingBuilder(t)
	for _, name := range stagedFileNames {
		writeUserFile(t, dir, name)
	}
	if err := os.WriteFile(filepath.Join(dir, "user-notes.txt"), []byte("unrelated"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanup, err := b.generateAndWriteDockerfile(context.Background())
	if err != nil {
		t.Fatalf("generateAndWriteDockerfile() error = %v", err)
	}
	cleanup()

	assertUserFilesRestored(t, dir, stagedFileNames)
	data, err := os.ReadFile(filepath.Join(dir, "user-notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "unrelated" {
		t.Errorf("file staging never touched content = %q, want %q", string(data), "unrelated")
	}
}

// TestGenerateAndWriteDockerfile_RestoresOnStagingError pins the #734
// partial-error path: when staging fails midway, files it already replaced
// are restored and files it created are removed.
func TestGenerateAndWriteDockerfile_RestoresOnStagingError(t *testing.T) {
	b, dir := newStagingBuilder(t)
	// The user owns a wrapper binary (backed up when staging replaces it) and
	// blocks config.yaml with a directory so the staging write fails.
	writeUserFile(t, dir, stagedFileNames[0])
	if err := os.MkdirAll(filepath.Join(dir, "config.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	cleanup, err := b.generateAndWriteDockerfile(context.Background())
	if err == nil {
		t.Fatal("generateAndWriteDockerfile() expected config write error")
	}
	cleanup()

	assertUserFilesRestored(t, dir, []string{stagedFileNames[0]})
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil {
		t.Errorf("config.yaml path should keep the user's directory: %v", err)
	}
}

// TestGenerateAndWriteDockerfile_BackupDirCreationFails covers the staging
// setup error: when the backup directory cannot be created, staging fails
// before touching anything.
func TestGenerateAndWriteDockerfile_BackupDirCreationFails(t *testing.T) {
	stubCachedWrapper(t, "amd64")
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := NewBuilder(BuildOptions{ServerBuildDir: file}, runner.NewRunner(false, false))

	cleanup, err := b.generateAndWriteDockerfile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "creating staging backup directory") {
		t.Fatalf("generateAndWriteDockerfile() error = %v, want backup dir failure", err)
	}
	cleanup()
}

// TestGenerateAndWriteDockerfile_BackupRenameFails covers the backup error
// path: when a pre-existing user file cannot be moved aside, staging fails
// without replacing it.
func TestGenerateAndWriteDockerfile_BackupRenameFails(t *testing.T) {
	b, dir := newStagingBuilder(t)
	// A user-owned Dockerfile file, plus a directory occupying the backup
	// slot for its name, makes the backup rename fail.
	writeUserFile(t, dir, "Dockerfile")
	if err := os.MkdirAll(filepath.Join(dir, stagingBackupDirName, "Dockerfile"), 0o755); err != nil {
		t.Fatal(err)
	}

	cleanup, err := b.generateAndWriteDockerfile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "backing up") {
		t.Fatalf("generateAndWriteDockerfile() error = %v, want backup failure", err)
	}
	cleanup()

	data, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil || string(data) != "user-owned Dockerfile" {
		t.Errorf("user Dockerfile must be untouched, got %q err=%v", string(data), err)
	}
}

// TestGenerateAndWriteDockerfile_CopyFailsAfterBackup covers the copy error
// path after a successful backup: the user file is still restored.
func TestGenerateAndWriteDockerfile_CopyFailsAfterBackup(t *testing.T) {
	b, dir := newStagingBuilder(t)
	writeUserFile(t, dir, stagedFileNames[0])
	// Make the cached wrapper path a directory so copying it fails after the
	// user's file at the destination was backed up.
	cacheDir, err := wrapper.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	bin := wrapper.BinaryPath(cacheDir, "linux", "amd64")
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}

	cleanup, err := b.generateAndWriteDockerfile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "copying wrapper binary") {
		t.Fatalf("generateAndWriteDockerfile() error = %v, want wrapper copy failure", err)
	}
	cleanup()
	assertUserFilesRestored(t, dir, []string{stagedFileNames[0]})
}

// TestBuild_DefersRestoreOnError verifies Build runs the staging restore even
// when staging fails, so user files never survive in a clobbered state (#734).
func TestBuild_DefersRestoreOnError(t *testing.T) {
	b, dir := newStagingBuilder(t)
	// User owns wrapper + config + dockerignore; the Dockerfile path is a
	// directory so staging fails after the first two files were replaced.
	writeUserFile(t, dir, stagedFileNames[0])
	writeUserFile(t, dir, "config.yaml")
	writeUserFile(t, dir, stagedFileNames[3])
	if err := os.MkdirAll(filepath.Join(dir, "Dockerfile"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := b.Build(context.Background()); err == nil {
		t.Fatal("Build() expected staging error")
	}
	assertUserFilesRestored(t, dir, []string{stagedFileNames[0], "config.yaml", stagedFileNames[3]})
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err != nil {
		t.Errorf("Dockerfile path should keep the user's directory: %v", err)
	}
}
