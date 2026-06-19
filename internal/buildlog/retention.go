package buildlog

import (
	"os"
	"path/filepath"
	"sort"
)

// Prune deletes the oldest *.log files in dir, keeping the newest keep files
// (by modification time). A missing directory or keep <= 0 is a no-op-ish:
// a missing dir returns nil; keep <= 0 leaves files untouched.
func Prune(dir string, keep int) error {
	if keep <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	type logFile struct {
		path    string
		modTime int64
	}
	var logs []logFile
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".log" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		logs = append(logs, logFile{path: filepath.Join(dir, e.Name()), modTime: info.ModTime().UnixNano()})
	}

	if len(logs) <= keep {
		return nil
	}

	// Newest first; delete everything past the keep count.
	sort.Slice(logs, func(i, j int) bool { return logs[i].modTime > logs[j].modTime })
	for _, l := range logs[keep:] {
		if err := os.Remove(l.path); err != nil {
			return err
		}
	}
	return nil
}
