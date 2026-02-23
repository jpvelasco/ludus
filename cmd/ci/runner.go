package ci

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/devrecon/ludus/cmd/globals"
	internalci "github.com/devrecon/ludus/internal/ci"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/spf13/cobra"
)

var (
	runnerDir    string
	runnerLabels string
	runnerName   string
	runnerRepo   string
	serviceFlag  bool
	deleteFlag   bool
)

var runnerCmd = &cobra.Command{
	Use:   "runner",
	Short: "Manage the self-hosted GitHub Actions runner",
	Long: `Commands for installing, checking, and removing a GitHub Actions
self-hosted runner agent on the current machine.

Runner management is Linux-only. The runner agent runs on the same
machine that has UE5 built from source.`,
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and configure the self-hosted runner",
	Long: `Downloads the GitHub Actions runner agent, registers it with the repository,
and optionally installs it as a systemd service.

Requires the gh CLI to be installed and authenticated.`,
	RunE: runInstall,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check runner agent status",
	RunE:  runStatus,
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the runner agent",
	Long: `Deregisters the runner from GitHub and optionally deletes the install directory.

Use --delete to also remove the install directory.`,
	RunE: runUninstall,
}

func init() {
	installCmd.Flags().StringVar(&runnerDir, "dir", "", "install directory (default: from config or ~/actions-runner)")
	installCmd.Flags().StringVar(&runnerLabels, "labels", "", "runner labels, comma-separated (default: self-hosted,linux,x64)")
	installCmd.Flags().StringVar(&runnerName, "name", "", "runner name (default: ludus-<hostname>)")
	installCmd.Flags().StringVar(&runnerRepo, "repo", "", "GitHub repo owner/name (default: auto-detect from git remote)")
	installCmd.Flags().BoolVar(&serviceFlag, "service", false, "install as systemd service (requires sudo)")

	uninstallCmd.Flags().StringVar(&runnerRepo, "repo", "", "GitHub repo owner/name (default: auto-detect from git remote)")
	uninstallCmd.Flags().BoolVar(&deleteFlag, "delete", false, "also delete the install directory")

	statusCmd.Flags().StringVar(&runnerDir, "dir", "", "install directory (default: from config or ~/actions-runner)")

	runnerCmd.AddCommand(installCmd)
	runnerCmd.AddCommand(statusCmd)
	runnerCmd.AddCommand(uninstallCmd)
}

func resolveRepo(ctx context.Context, r *runner.Runner) (string, error) {
	if runnerRepo != "" {
		return runnerRepo, nil
	}

	// Auto-detect from git remote
	out, err := r.RunOutput(ctx, "git", "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("cannot auto-detect repo (use --repo flag): %w", err)
	}
	return internalci.ParseRepoFromRemote(string(out))
}

func resolveDir() string {
	if runnerDir != "" {
		return runnerDir
	}
	return globals.Cfg.CI.RunnerDir
}

func resolveLabels() string {
	if runnerLabels != "" {
		return runnerLabels
	}
	labels := globals.Cfg.CI.RunnerLabels
	if len(labels) == 0 {
		return "self-hosted,linux,x64"
	}
	return strings.Join(labels, ",")
}

func resolveName() string {
	if runnerName != "" {
		return runnerName
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return "ludus-" + hostname
}

func runInstall(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("runner install is only supported on Linux")
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	repo, err := resolveRepo(cmd.Context(), r)
	if err != nil {
		return err
	}

	installer := &internalci.RunnerInstaller{
		Runner:     r,
		InstallDir: resolveDir(),
		Labels:     resolveLabels(),
		Name:       resolveName(),
		Repo:       repo,
		Service:    serviceFlag,
	}

	return installer.Install(cmd.Context())
}

func runStatus(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("runner status is only supported on Linux")
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	installer := &internalci.RunnerInstaller{
		Runner:     r,
		InstallDir: resolveDir(),
	}

	status, err := installer.Status(cmd.Context())
	if err != nil {
		return err
	}

	fmt.Printf("Runner status: %s\n", status)
	return nil
}

func runUninstall(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("runner uninstall is only supported on Linux")
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	repo, err := resolveRepo(cmd.Context(), r)
	if err != nil {
		return err
	}

	installer := &internalci.RunnerInstaller{
		Runner:     r,
		InstallDir: resolveDir(),
		Repo:       repo,
	}

	return installer.Uninstall(cmd.Context(), deleteFlag)
}
