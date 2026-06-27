package ddc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Clean removes all contents of dir (not dir itself) and returns bytes freed.
// Returns 0 without error if dir doesn't exist.
func Clean(dir string) (int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading DDC directory: %w", err)
	}
	var freed int64
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		freed += entrySize(path, entry)
		if err := os.RemoveAll(path); err != nil {
			return freed, fmt.Errorf("removing %s: %w", entry.Name(), err)
		}
	}
	return freed, nil
}

// Prune removes files older than maxAgeDays and returns bytes freed.
// Returns 0 without error if dir doesn't exist.
// maxAgeDays must be at least 1 to prevent accidental deletion of all files.
func Prune(dir string, maxAgeDays int) (int64, error) {
	if maxAgeDays < 1 {
		return 0, fmt.Errorf("max age must be at least 1 day (got %d)", maxAgeDays)
	}
	cutoff := time.Now().Add(-time.Duration(maxAgeDays) * 24 * time.Hour)
	freed, err := removeOldFiles(dir, cutoff)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	return freed, err
}

// removeOldFiles walks dir and removes files with modtime before cutoff.
func removeOldFiles(dir string, cutoff time.Time) (int64, error) {
	var freed int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // entry vanished between readdir and visit
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		return pruneIfOld(path, d, cutoff, &freed)
	})
	return freed, err
}

// pruneIfOld removes a single file if its modtime is before cutoff,
// accumulating freed bytes. Files that disappear mid-walk are skipped silently.
func pruneIfOld(path string, d fs.DirEntry, cutoff time.Time, freed *int64) error {
	info, err := d.Info()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // file vanished between WalkDir discovering it and Info()
		}
		return err
	}
	if !info.ModTime().Before(cutoff) {
		return nil
	}
	size := info.Size()
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // already removed by another process
		}
		return err
	}
	*freed += size
	return nil
}
