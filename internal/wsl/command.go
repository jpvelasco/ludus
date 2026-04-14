package wsl

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/internal/runner"
)

const wslExe = "wsl.exe"

// Run executes a command inside the given WSL2 distro via wsl.exe -d.
func Run(ctx context.Context, r *runner.Runner, distro string, args ...string) error {
	wslArgs := buildArgs(distro, args)
	return r.Run(ctx, wslExe, wslArgs...)
}

// RunBash executes a bash script string inside the given WSL2 distro.
// The script is passed as: wsl.exe -d <distro> bash -c "<script>"
func RunBash(ctx context.Context, r *runner.Runner, distro, script string) error {
	wslArgs := buildBashArgs(distro, script)
	return r.Run(ctx, wslExe, wslArgs...)
}

// RunOutput executes a command inside the given WSL2 distro and returns stdout.
func RunOutput(ctx context.Context, r *runner.Runner, distro string, args ...string) ([]byte, error) {
	wslArgs := buildArgs(distro, args)
	return r.RunOutput(ctx, wslExe, wslArgs...)
}

// buildArgs constructs the wsl.exe argument list for direct command execution.
func buildArgs(distro string, args []string) []string {
	wslArgs := make([]string, 0, 4+len(args))
	wslArgs = append(wslArgs, "-d", distro, "-e")
	wslArgs = append(wslArgs, args...)
	return wslArgs
}

// buildBashArgs constructs the wsl.exe argument list for bash -c execution.
func buildBashArgs(distro, script string) []string {
	return []string{"-d", distro, "bash", "-c", script}
}

// RunBashAsRoot executes a bash script as root inside the given WSL2 distro.
// Uses wsl.exe -u root, which bypasses sudo entirely and works even when
// sudo requires a password (common in default WSL2 installs).
func RunBashAsRoot(ctx context.Context, r *runner.Runner, distro, script string) error {
	wslArgs := []string{"-d", distro, "-u", "root", "bash", "-c", script}
	return r.Run(ctx, wslExe, wslArgs...)
}

// RunSudo executes a command with sudo inside the given WSL2 distro.
func RunSudo(ctx context.Context, r *runner.Runner, distro string, args ...string) error {
	sudoArgs := make([]string, 0, 1+len(args))
	sudoArgs = append(sudoArgs, "sudo")
	sudoArgs = append(sudoArgs, args...)
	return Run(ctx, r, distro, sudoArgs...)
}

// CheckCommand verifies that a command exists inside the WSL2 distro.
// Returns nil if the command is found, an error otherwise.
func CheckCommand(ctx context.Context, r *runner.Runner, distro, cmd string) error {
	_, err := RunOutput(ctx, r, distro, "which", cmd)
	if err != nil {
		return fmt.Errorf("%s not found in WSL2 distro %q", cmd, distro)
	}
	return nil
}
