package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
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

// environ returns the merged environment for child processes. If Env is empty,
// it returns nil so exec.Cmd inherits the parent environment unchanged.
// When Env is set, parent variables are included and any matching keys are
// overridden by the Env values.
func (r *Runner) environ() []string {
	if len(r.Env) == 0 {
		return nil
	}

	// Build a set of override keys for quick lookup.
	overrides := make(map[string]string, len(r.Env))
	for _, kv := range r.Env {
		if k, _, ok := strings.Cut(kv, "="); ok {
			overrides[k] = kv
		}
	}

	// Start with parent env, replacing any keys present in overrides.
	parent := os.Environ()
	merged := make([]string, 0, len(parent)+len(r.Env))
	seen := make(map[string]bool, len(overrides))
	for _, kv := range parent {
		k, _, _ := strings.Cut(kv, "=")
		if override, ok := overrides[k]; ok {
			merged = append(merged, override)
			seen[k] = true
		} else {
			merged = append(merged, kv)
		}
	}

	// Append any override keys that weren't in the parent env.
	for k, kv := range overrides {
		if !seen[k] {
			merged = append(merged, kv)
		}
	}

	return merged
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
	cmd.Env = r.environ()
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	return cmd.Run()
}

// RunOutput executes a command and returns its stdout as bytes instead of streaming.
func (r *Runner) RunOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.Verbose || r.DryRun {
		fmt.Fprintf(r.Stdout, "+ %s", name)
		for _, arg := range args {
			fmt.Fprintf(r.Stdout, " %s", arg)
		}
		fmt.Fprintln(r.Stdout)
	}

	if r.DryRun {
		return []byte("(dry-run)"), nil
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = r.environ()
	cmd.Stderr = r.Stderr
	return cmd.Output()
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
	cmd.Env = r.environ()
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	return cmd.Run()
}
