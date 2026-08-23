package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
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

// RunQuietErr behaves like RunQuiet but, on failure, wraps the exit error
// with the captured stderr so callers can classify AWS CLI errors (e.g.
// RepositoryAlreadyExistsException, AccessDenied) from err.Error().
func (r *Runner) RunQuietErr(ctx context.Context, name string, args ...string) error {
	if r.echo(name, args) {
		return nil
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = r.environ()
	if r.Verbose {
		cmd.Stdout = r.Stdout
	}
	var stderr bytes.Buffer
	cmd.Stderr = io.MultiWriter(r.Stderr, &stderr)
	if err := cmd.Run(); err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return fmt.Errorf("%w: %s", err, detail)
		}
		return err
	}
	return nil
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
