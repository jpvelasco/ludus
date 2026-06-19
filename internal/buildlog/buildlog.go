// Package buildlog persists ludus build output to timestamped files under a log
// directory (default .ludus/logs). A Logger owns one log file for the duration
// of a command invocation; callers tee command output to Logger.Writer() via an
// io.MultiWriter so the terminal output is unchanged and a durable copy is kept.
package buildlog

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Logger owns a single build log file.
type Logger struct {
	path string
	f    *os.File
}

// filePrefix marks files this package owns, so retention never touches unrelated
// *.log files in a shared log directory.
const filePrefix = "ludus-"

// New creates the log directory if needed and opens a uniquely-named log file
// "ludus-<timestamp>-<runName>.log". The timestamp is supplied by the caller so
// the package stays clock-free and testable. If a file with the same name
// already exists (two invocations within the same second), a numeric suffix is
// added so an earlier log is never truncated.
func New(dir, runName string, now time.Time) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating log dir %s: %w", dir, err)
	}
	stamp := now.Format("2006-01-02T15-04-05")
	for attempt := 0; ; attempt++ {
		name := fmt.Sprintf("%s%s-%s.log", filePrefix, stamp, runName)
		if attempt > 0 {
			name = fmt.Sprintf("%s%s-%s.%d.log", filePrefix, stamp, runName, attempt)
		}
		path := filepath.Join(dir, name)
		// O_EXCL fails if the file exists, so we never clobber an earlier log.
		// 0644: build logs are non-secret and must be readable by the non-root
		// container user that runs MCP/container builds.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644) //nolint:gosec // G302: 0644 intentional, see comment
		if err == nil {
			return &Logger{path: path, f: f}, nil
		}
		if os.IsExist(err) && attempt < 1000 {
			continue
		}
		return nil, fmt.Errorf("creating log file %s: %w", path, err)
	}
}

// Path returns the absolute (or relative-as-given) path to the log file.
func (l *Logger) Path() string { return l.path }

// Writer returns the underlying file as an io.Writer for teeing command output.
func (l *Logger) Writer() *os.File { return l.f }

// Section writes a delimited header so a single per-run log reads as ordered
// stage sections (e.g. "===== Build Unreal Engine (15:04:05) =====").
func (l *Logger) Section(name string) {
	if l == nil || l.f == nil {
		return
	}
	fmt.Fprintf(l.f, "\n===== %s (%s) =====\n", name, time.Now().Format("15:04:05"))
}

// Close closes the log file.
func (l *Logger) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	return l.f.Close()
}
