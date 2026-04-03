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

// IniSection returns the DerivedDataBackendGraph ini block pointing to ddcPath.
// The returned string includes a leading newline and the full section.
func IniSection(ddcPath string) string {
	// UE5 ini files use forward slashes even on Windows.
	p := filepath.ToSlash(ddcPath)
	return fmt.Sprintf("\n[DerivedDataBackendGraph]\nDefault=Async\nAsync=(Type=FileSystem, Root=\"%s\", ReadOnly=false)\n", p)
}

// PatchProjectINI adds DDC configuration to a project's DefaultEngine.ini.
// Returns a restore function that reverts the file to its original content.
// The caller should defer the restore function.
// If the ini already contains a DerivedDataBackendGraph section, it is left unchanged.
func PatchProjectINI(iniPath, ddcPath string) (restore func(), err error) {
	noop := func() {}

	data, err := os.ReadFile(iniPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return noop, nil
		}
		return noop, fmt.Errorf("reading %s: %w", iniPath, err)
	}

	content := string(data)
	if strings.Contains(content, "DerivedDataBackendGraph") {
		return noop, nil
	}

	patched := content + IniSection(ddcPath)
	if err := os.WriteFile(iniPath, []byte(patched), 0644); err != nil {
		return noop, fmt.Errorf("writing DDC config to %s: %w", iniPath, err)
	}

	fmt.Printf("  DDC: configured persistent cache at %s\n", ddcPath)
	return func() {
		if err := os.WriteFile(iniPath, data, 0644); err != nil {
			fmt.Printf("  WARNING: failed to restore %s: %v\n", iniPath, err)
			fmt.Printf("  Your DefaultEngine.ini may contain injected DDC config.\n")
			fmt.Printf("  To fix: git checkout -- %s\n", iniPath)
		}
	}, nil
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
func ResolvePath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return DefaultPath()
}

// DirSize returns the total bytes of all files under dir.
// Returns 0 without error if dir doesn't exist.
func DirSize(dir string) (int64, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
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
		if errors.Is(err, fs.ErrNotExist) {
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
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
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
