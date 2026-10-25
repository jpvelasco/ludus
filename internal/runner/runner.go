package runner

import (
	"fmt"
	"io"
	"os"
)

// Runner executes shell commands with output streaming and error handling.
type Runner struct {
	// Stdout is where command stdout is written. Defaults to os.Stdout.
	Stdout io.Writer
	// Stderr is where command stderr is written. Defaults to os.Stderr.
	Stderr io.Writer
	// Verbose enables printing the command before execution.
	Verbose bool
	// DryRun prints commands without executing them.
	DryRun bool
	// Env holds extra environment variables (KEY=VALUE) to set on child
	// processes. These are merged on top of the parent process environment,
	// overriding any existing variables with the same key.
	Env []string
}

// NewRunner creates a new command runner.
func NewRunner(verbose, dryRun bool) *Runner {
	return &Runner{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Verbose: verbose,
		DryRun:  dryRun,
	}
}

// redactedArgValue replaces the value of a sensitive argument in echo output.
const redactedArgValue = "********"

// sensitiveArgs are argument names whose values must never appear in verbose
// or dry-run echo output (e.g. GitHub Actions runner registration tokens).
// The command itself still receives the real value; only the echoed line is
// redacted.
var sensitiveArgs = map[string]bool{"--token": true}

// echo prints the command line if verbose or dry-run and returns true if dry-run.
// Callers should return early when echo returns true. Values of sensitive
// arguments are redacted from the echoed line only (#708).
func (r *Runner) echo(prefix string, args []string) bool {
	if r.Verbose || r.DryRun {
		fmt.Fprintf(r.Stdout, "+ %s", prefix)
		for _, arg := range redactArgs(args) {
			fmt.Fprintf(r.Stdout, " %s", arg)
		}
		fmt.Fprintln(r.Stdout)
	}
	return r.DryRun
}

// redactArgs returns a copy of args with the value following each sensitive
// argument name replaced by a placeholder. Execution args are untouched; only
// diagnostics are redacted.
func redactArgs(args []string) []string {
	redacted := make([]string, len(args))
	copy(redacted, args)
	for i := 0; i+1 < len(redacted); i++ {
		if sensitiveArgs[redacted[i]] {
			redacted[i+1] = redactedArgValue
		}
	}
	return redacted
}
