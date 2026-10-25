package container

import (
	"fmt"
	"os"
	"path/filepath"
)

// stagingBackupDirName is the directory (inside the server build directory)
// where staging parks pre-existing user files it must replace while the
// container image builds.
const stagingBackupDirName = ".ludus-staging-backup"

// stagedFiles stages generated files (wrapper binary, wrapper config,
// Dockerfile, .dockerignore) into the packaged server directory — which the
// user owns. Any pre-existing file is moved aside first, and restore puts it
// back exactly as it was, so cleanup never deletes or clobbers files the
// staging did not create (#734).
type stagedFiles struct {
	backupDir string
	created   []string          // paths staging wrote
	backups   map[string]string // original path -> backup path
}

// newStagedFiles creates the staging tracker and its backup directory.
func newStagedFiles(dir string) (*stagedFiles, error) {
	backupDir := filepath.Join(dir, stagingBackupDirName)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating staging backup directory: %w", err)
	}
	return &stagedFiles{backupDir: backupDir, backups: map[string]string{}}, nil
}

// backupFile moves a pre-existing regular file out of staging's way.
// Directories and missing paths are left alone so writes fail exactly as
// they would without staging.
func (s *stagedFiles) backupFile(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	backup := filepath.Join(s.backupDir, filepath.Base(path))
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("backing up %s: %w", path, err)
	}
	s.backups[path] = backup
	return nil
}

// writeFile backs up any pre-existing file at path and replaces it with
// content, recording the written path for cleanup.
func (s *stagedFiles) writeFile(path string, content []byte, perm os.FileMode) error {
	return s.replace(path, func() error { return os.WriteFile(path, content, perm) })
}

// copyFile backs up any pre-existing file at dst and copies src over it,
// recording dst for cleanup.
func (s *stagedFiles) copyFile(src, dst string) error {
	return s.replace(dst, func() error { return copyFile(src, dst) })
}

// replace backs up whatever occupies path, writes the new content there, and
// records the path so restore removes only what staging wrote.
func (s *stagedFiles) replace(path string, write func() error) error {
	if err := s.backupFile(path); err != nil {
		return err
	}
	if err := write(); err != nil {
		return err
	}
	s.created = append(s.created, path)
	return nil
}

// restore removes every file staging created, puts every backed-up user file
// back in place, and removes the backup directory. It runs on success and on
// failure alike, so the packaged directory always ends in its pre-staging
// state (#734). Errors are ignored: the files live in a temp-like build
// directory and a failed best-effort restore must not mask the build error.
func (s *stagedFiles) restore() {
	for _, path := range s.created {
		os.Remove(path)
	}
	for path, backup := range s.backups {
		_ = os.Rename(backup, path) // best-effort restore; see doc comment
	}
	os.RemoveAll(s.backupDir)
}
