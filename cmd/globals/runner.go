package globals

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/jpvelasco/ludus/internal/buildlog"
	"github.com/jpvelasco/ludus/internal/runner"
)

// NoLogs disables on-disk build logging (set via --no-logs), overriding config.
var NoLogs bool

// CommandName is the invoked subcommand (e.g. "engine", "run"), used to name the
// log file. Set in root PersistentPreRunE.
var CommandName string

// buildLog is the process's active log file (CLI mode). It is opened lazily on
// the first NewRunner() call so non-build commands never create a log file.
var (
	buildLog     *buildlog.Logger
	buildLogOnce sync.Once
)

// NewRunner constructs a runner.Runner wired to the global Verbose/DryRun flags.
// On its first call (for build commands), it lazily opens a per-invocation log
// file under the configured logs dir and tees stdout/stderr to it. Subsequent
// calls reuse the same log file so a whole `ludus run` lands in one file.
func NewRunner() *runner.Runner {
	r := runner.NewRunner(Verbose, DryRun)
	if sink := buildLogSink(); sink != nil {
		r.Stdout = io.MultiWriter(os.Stdout, sink)
		r.Stderr = io.MultiWriter(os.Stderr, sink)
	}
	return r
}

// buildLogSink returns the active log file writer, opening it once. Returns nil
// when logging is disabled (config, --no-logs, dry-run, or open failure).
func buildLogSink() io.Writer {
	buildLogOnce.Do(func() {
		if NoLogs || DryRun || Cfg == nil || !Cfg.Observability.Logs.IsEnabled() {
			return
		}
		dir := Cfg.Observability.Logs.Dir
		if dir == "" {
			dir = ".ludus/logs"
		}
		name := CommandName
		if name == "" {
			name = "ludus"
		}
		lg, err := buildlog.New(dir, name, time.Now())
		if err != nil {
			// Logging is best-effort; never block a build on a log-open failure.
			return
		}
		buildLog = lg
		_ = buildlog.Prune(dir, Cfg.Observability.Logs.RetainRuns)
	})
	if buildLog == nil {
		return nil
	}
	return buildLog.Writer()
}

// LogsDir returns the configured build-logs directory (default ".ludus/logs").
func LogsDir() string {
	if Cfg != nil && Cfg.Observability.Logs.Dir != "" {
		return Cfg.Observability.Logs.Dir
	}
	return ".ludus/logs"
}

// SectionLog writes a stage section header to the active build log, if any.
func SectionLog(name string) {
	if buildLog != nil {
		buildLog.Section(name)
	}
}

// CloseBuildLog closes the active build log file. Call once on process exit.
func CloseBuildLog() {
	if buildLog != nil {
		_ = buildLog.Close()
		buildLog = nil
	}
}

// resetBuildLogOnce resets the lazy-open guard. Test-only helper.
func resetBuildLogOnce() {
	buildLogOnce = sync.Once{}
}
