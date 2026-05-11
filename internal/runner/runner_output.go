package runner

import (
	"context"
	"io"
	"os/exec"
)

// RunOutput executes a command and returns its stdout as bytes instead of streaming.
func (r *Runner) RunOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.echo(name, args) {
		return []byte("(dry-run)"), nil
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = r.environ()
	cmd.Stderr = r.Stderr
	return cmd.Output()
}

// RunQuiet executes a command, suppressing stdout unless Verbose is set.
// Stderr is always shown. Use this for commands whose stdout contains
// sensitive data (e.g. AWS account IDs, tokens) that should not be
// printed in normal mode.
func (r *Runner) RunQuiet(ctx context.Context, name string, args ...string) error {
	if r.echo(name, args) {
		return nil
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = r.environ()
	if r.Verbose {
		cmd.Stdout = r.Stdout
	}
	cmd.Stderr = r.Stderr
	return cmd.Run()
}

// RunQuietWithStdin executes a command with the given reader piped to stdin,
// suppressing stdout unless Verbose is set. Stderr is always shown.
func (r *Runner) RunQuietWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) error {
	if r.echo(name, args) {
		return nil
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = r.environ()
	cmd.Stdin = stdin
	if r.Verbose {
		cmd.Stdout = r.Stdout
	}
	cmd.Stderr = r.Stderr
	return cmd.Run()
}
