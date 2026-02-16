package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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

// Run executes a command and streams its output.
func (r *Runner) Run(ctx context.Context, name string, args ...string) error {
	if r.Verbose || r.DryRun {
		fmt.Fprintf(r.Stdout, "+ %s", name)
		for _, arg := range args {
			fmt.Fprintf(r.Stdout, " %s", arg)
		}
		fmt.Fprintln(r.Stdout)
	}

	if r.DryRun {
		return nil
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	return cmd.Run()
}

// RunInDir executes a command in a specific directory.
func (r *Runner) RunInDir(ctx context.Context, dir, name string, args ...string) error {
	if r.Verbose || r.DryRun {
		fmt.Fprintf(r.Stdout, "+ cd %s && %s", dir, name)
		for _, arg := range args {
			fmt.Fprintf(r.Stdout, " %s", arg)
		}
		fmt.Fprintln(r.Stdout)
	}

	if r.DryRun {
		return nil
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	return cmd.Run()
}
