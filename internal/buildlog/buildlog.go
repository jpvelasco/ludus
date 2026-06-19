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

// New creates the log directory if needed and opens a timestamped log file named
// "<RFC3339-ish timestamp>-<runName>.log". The timestamp is supplied by the
// caller so the package stays clock-free and testable.
func New(dir, runName string, now time.Time) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating log dir %s: %w", dir, err)
	}
	name := fmt.Sprintf("%s-%s.log", now.Format("2006-01-02T15-04-05"), runName)
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("creating log file %s: %w", path, err)
	}
	return &Logger{path: path, f: f}, nil
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
