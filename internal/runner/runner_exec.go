package runner

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// Run executes a command and streams its output.
func (r *Runner) Run(ctx context.Context, name string, args ...string) error {
	if r.echo(name, args) {
		return nil
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = r.environ()
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	return cmd.Run()
}

// RunWithStdin executes a command with the given reader piped to stdin.
func (r *Runner) RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) error {
	if r.echo(name, args) {
		return nil
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = r.environ()
	cmd.Stdin = stdin
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	return cmd.Run()
}

// RunInDir executes a command in a specific directory.
func (r *Runner) RunInDir(ctx context.Context, dir, name string, args ...string) error {
	if r.echo(fmt.Sprintf("cd %s && %s", dir, name), args) {
		return nil
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = r.environ()
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	return cmd.Run()
}
