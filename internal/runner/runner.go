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

// echo prints the command line if verbose or dry-run and returns true if dry-run.
// Callers should return early when echo returns true.
func (r *Runner) echo(prefix string, args []string) bool {
	if r.Verbose || r.DryRun {
		fmt.Fprintf(r.Stdout, "+ %s", prefix)
		for _, arg := range args {
			fmt.Fprintf(r.Stdout, " %s", arg)
		}
		fmt.Fprintln(r.Stdout)
	}
	return r.DryRun
}
