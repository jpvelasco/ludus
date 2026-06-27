package ddc

import (
	"errors"
	"io/fs"
	"path/filepath"
)

// DirSize returns the total bytes of all files under dir.
// Returns 0 without error if dir doesn't exist.
// Files that vanish mid-walk (e.g. evicted by the engine) are skipped silently.
func DirSize(dir string) (int64, error) {
	if dir == "" {
		return 0, nil
	}
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // file/dir vanished between readdir and stat
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // file vanished between WalkDir discovering it and Info()
			}
			return err
		}
		total += info.Size()
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	return total, err
}

// entrySize returns the size of an entry. For files it returns the file size;
// for directories it walks recursively to sum all file sizes.
func entrySize(path string, d fs.DirEntry) int64 {
	if !d.IsDir() {
		info, err := d.Info()
		if err != nil {
			return 0
		}
		return info.Size()
	}
	var total int64
	_ = filepath.WalkDir(path, func(_ string, wd fs.DirEntry, err error) error {
		if err != nil || wd.IsDir() {
			return err
		}
		info, err := wd.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}
