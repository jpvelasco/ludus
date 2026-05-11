package cache

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// gitHEAD returns the git HEAD commit hash for the given directory.
// Returns empty string if git is not available or the directory is not a repo.
func gitHEAD(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// fileKey returns "mtime:size" for a file, or empty string if stat fails.
func fileKey(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
}

// dirManifest returns a deterministic string of "name:size" entries
// for all files in a directory tree, sorted by path.
func dirManifest(dir string) string {
	var entries []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		entries = append(entries, fmt.Sprintf("%s:%d", rel, info.Size()))
		return nil
	})
	sort.Strings(entries)
	return strings.Join(entries, "\n")
}
