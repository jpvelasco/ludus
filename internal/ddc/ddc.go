package ddc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// ValidateDDCMode returns the normalized mode or an error for unknown values.
// Empty string is normalized to "local" (the default).
func ValidateDDCMode(mode string) (string, error) {
	switch mode {
	case "", "local":
		return "local", nil
	case "none":
		return "none", nil
	default:
		return "", fmt.Errorf("invalid DDC mode %q: valid values are \"local\" (persistent cache, default) or \"none\" (disable cache)", mode)
	}
}

// IniOverrideArgs returns RunUAT command-line arguments that configure DDC
// via UE5's -ini: override mechanism. This avoids modifying any project files.
func IniOverrideArgs(ddcPath string) []string {
	// UE5 ini files use forward slashes even on Windows.
	p := filepath.ToSlash(ddcPath)
	return []string{
		"-ini:Engine:[DerivedDataBackendGraph]:Default=Async",
		fmt.Sprintf(`-ini:Engine:[DerivedDataBackendGraph]:Async=(Type=FileSystem, Root="%s", ReadOnly=false)`, p),
	}
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
func DirSize(dir string) (int64, error) {
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("checking DDC directory: %w", err)
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
	if err := removeContents(dir); err != nil {
		return 0, err
	}
	return size, nil
}

// removeContents deletes all entries in dir without removing dir itself.
func removeContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading DDC directory: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return fmt.Errorf("removing %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// Prune removes files older than maxAgeDays and returns bytes freed.
// Returns 0 without error if dir doesn't exist.
// maxAgeDays must be at least 1 to prevent accidental deletion of all files.
func Prune(dir string, maxAgeDays int) (int64, error) {
	if maxAgeDays < 1 {
		return 0, fmt.Errorf("max age must be at least 1 day (got %d)", maxAgeDays)
	}
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("checking DDC directory: %w", err)
	}
	cutoff := time.Now().Add(-time.Duration(maxAgeDays) * 24 * time.Hour)
	return removeOldFiles(dir, cutoff)
}

// removeOldFiles walks dir and removes files with modtime before cutoff.
func removeOldFiles(dir string, cutoff time.Time) (int64, error) {
	var freed int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return pruneIfOld(path, d, cutoff, &freed)
	})
	return freed, err
}

// pruneIfOld removes a single file if its modtime is before cutoff,
// accumulating freed bytes.
func pruneIfOld(path string, d fs.DirEntry, cutoff time.Time, freed *int64) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	if !info.ModTime().Before(cutoff) {
		return nil
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	*freed += info.Size()
	return nil
}
