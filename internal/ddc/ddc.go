package ddc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DDC mode constants. Use these instead of raw string comparisons.
const (
	ModeLocal = "local" // Persistent local filesystem cache (default).
	ModeNone  = "none"  // DDC disabled; no cache volume mounted.
)

// ValidateDDCMode returns the normalized mode or an error for unknown values.
// Empty string is normalized to ModeLocal (the default).
func ValidateDDCMode(mode string) (string, error) {
	switch mode {
	case "", ModeLocal:
		return ModeLocal, nil
	case ModeNone:
		return ModeNone, nil
	default:
		return "", fmt.Errorf("invalid DDC mode %q: valid values are %q (persistent cache, default) or %q (disable cache)", mode, ModeLocal, ModeNone)
	}
}

// EnvOverride returns the environment variable string that redirects UE5's
// local DDC backend to path. UE5's BaseEngine.ini configures the Local backend
// with EnvPathOverride=UE-LocalDataCachePath, so setting this env var overrides
// the default path without modifying any project or engine files.
//
// Uses strings.ReplaceAll instead of filepath.ToSlash because ToSlash is a
// no-op on Linux (backslash is a valid filename char, not a separator), but
// Windows paths passed here may still contain backslashes that Docker and
// UE5 need converted to forward slashes.
func EnvOverride(path string) string {
	normalized := strings.ReplaceAll(path, `\`, "/")
	return fmt.Sprintf("UE-LocalDataCachePath=%s", normalized)
}

// DefaultPath returns the default DDC directory path: ~/.ludus/ddc
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".ludus", "ddc"), nil
}

// ResolvePath returns the override path if non-empty, otherwise returns DefaultPath.
// Returns an error if the override is a relative path.
func ResolvePath(override string) (string, error) {
	if override != "" {
		if !filepath.IsAbs(override) {
			return "", fmt.Errorf("DDC path must be absolute (got %q); use a full path like /home/user/.ludus/ddc", override)
		}
		return override, nil
	}
	return DefaultPath()
}

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
