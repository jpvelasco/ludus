package ddc

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// DefaultPath returns the default DDC directory path: ~/.ludus/ddc
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".ludus", "ddc"), nil
}

// ResolvePath returns the override path if non-empty, otherwise returns DefaultPath.
func ResolvePath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return DefaultPath()
}

// DirSize returns the total bytes of all files under dir.
// Returns 0 without error if dir doesn't exist.
func DirSize(dir string) (int64, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, nil
	}
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// Clean removes all contents of dir (not dir itself) and returns bytes freed.
// Returns 0 without error if dir doesn't exist.
func Clean(dir string) (int64, error) {
	size, err := DirSize(dir)
	if err != nil {
		return 0, err
	}
	if size == 0 {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading DDC directory: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return 0, fmt.Errorf("removing %s: %w", entry.Name(), err)
		}
	}
	return size, nil
}

// Prune removes files older than maxAgeDays and returns bytes freed.
// Returns 0 without error if dir doesn't exist.
func Prune(dir string, maxAgeDays int) (int64, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, nil
	}
	cutoff := time.Now().Add(-time.Duration(maxAgeDays) * 24 * time.Hour)
	var freed int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(cutoff) {
			freed += info.Size()
			return os.Remove(path)
		}
		return nil
	})
	return freed, err
}
