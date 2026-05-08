package ci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/devrecon/ludus/internal/runner"
)

// RunnerInstaller manages the GitHub Actions self-hosted runner agent.
type RunnerInstaller struct {
	Runner *runner.Runner
	// InstallDir is the directory where the runner agent is installed.
	InstallDir string
	// Labels are the runner labels (e.g. "self-hosted,linux,x64").
	Labels string
	// Name is the runner name (defaults to "ludus-<hostname>").
	Name string
	// Repo is the GitHub repository in "owner/repo" format.
	Repo string
	// Service controls whether to install as a systemd service.
	Service bool
}

// Install downloads and configures the GitHub Actions runner agent.
func (ri *RunnerInstaller) Install(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("runner install is only supported on Linux")
	}

	dir := expandHome(ri.InstallDir)

	token, err := ri.registrationToken(ctx)
	if err != nil {
		return err
	}

	version, err := ri.latestRunnerVersion(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Installing runner %s to %s\n", version, dir)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating install directory: %w", err)
	}

	tarball, err := ri.downloadRunner(ctx, dir, version)
	if err != nil {
		return err
	}
	if err := ri.extractRunner(ctx, dir, tarball); err != nil {
		return err
	}
	if err := ri.configureRunner(ctx, dir, token); err != nil {
		return err
	}

	return ri.finishInstall(ctx, dir)
}

func (ri *RunnerInstaller) registrationToken(ctx context.Context) (string, error) {
	fmt.Println("Obtaining runner registration token...")
	tokenBytes, err := ri.Runner.RunOutput(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/actions/runners/registration-token", ri.Repo),
		"--method", "POST", "--jq", ".token")
	if err != nil {
		return "", fmt.Errorf("getting registration token (is gh authenticated?): %w", err)
	}
	return strings.TrimSpace(string(tokenBytes)), nil
}

func (ri *RunnerInstaller) latestRunnerVersion(ctx context.Context) (string, error) {
	fmt.Println("Fetching latest runner version...")
	versionBytes, err := ri.Runner.RunOutput(ctx, "gh", "api",
		"repos/actions/runner/releases/latest", "--jq", ".tag_name")
	if err != nil {
		return "", fmt.Errorf("getting runner version: %w", err)
	}
	return strings.TrimSpace(string(versionBytes)), nil
}

func (ri *RunnerInstaller) downloadRunner(ctx context.Context, dir, version string) (string, error) {
	versionNum := strings.TrimPrefix(version, "v")
	tarball := fmt.Sprintf("actions-runner-linux-x64-%s.tar.gz", versionNum)
	tarballPath := filepath.Join(dir, tarball)
	downloadURL := fmt.Sprintf("https://github.com/actions/runner/releases/download/%s/%s", version, tarball)

	fmt.Println("Downloading runner...")
	if err := ri.Runner.Run(ctx, "curl", "-o", tarballPath, "-L", downloadURL); err != nil {
		return "", fmt.Errorf("downloading runner: %w", err)
	}
	return tarball, nil
}

func (ri *RunnerInstaller) extractRunner(ctx context.Context, dir, tarball string) error {
	fmt.Println("Extracting runner...")
	if err := ri.Runner.RunInDir(ctx, dir, "tar", "xzf", tarball); err != nil {
		return fmt.Errorf("extracting runner: %w", err)
	}
	os.Remove(filepath.Join(dir, tarball))
	return nil
}

func (ri *RunnerInstaller) configureRunner(ctx context.Context, dir, token string) error {
	fmt.Println("Configuring runner...")
	configScript := filepath.Join(dir, "config.sh")
	if err := ri.Runner.RunInDir(ctx, dir, configScript, ri.configArgs(token)...); err != nil {
		return fmt.Errorf("configuring runner: %w", err)
	}
	return nil
}

func (ri *RunnerInstaller) configArgs(token string) []string {
	return []string{
		"--url", fmt.Sprintf("https://github.com/%s", ri.Repo),
		"--token", token,
		"--labels", ri.Labels,
		"--name", ri.Name,
		"--unattended",
		"--replace",
	}
}

func (ri *RunnerInstaller) finishInstall(ctx context.Context, dir string) error {
	if !ri.Service {
		fmt.Println("Runner configured. Start manually with: ./run.sh")
		return nil
	}

	fmt.Println("Installing systemd service...")
	svcScript := filepath.Join(dir, "svc.sh")
	if err := ri.Runner.RunInDir(ctx, dir, "sudo", svcScript, "install"); err != nil {
		return fmt.Errorf("installing service: %w", err)
	}
	if err := ri.Runner.RunInDir(ctx, dir, "sudo", svcScript, "start"); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}
	fmt.Println("Runner service installed and started.")
	return nil
}

// Status checks whether the runner agent is running.
func (ri *RunnerInstaller) Status(ctx context.Context) (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("runner status is only supported on Linux")
	}

	dir := expandHome(ri.InstallDir)

	// Check if install directory exists
	if _, err := os.Stat(filepath.Join(dir, "config.sh")); err != nil {
		return "not installed", nil
	}

	// Check systemd service
	svcScript := filepath.Join(dir, "svc.sh")
	if _, err := os.Stat(svcScript); err == nil {
		err := ri.Runner.RunInDir(ctx, dir, "sudo", svcScript, "status")
		if err == nil {
			return "running (systemd)", nil
		}
	}

	// Check for running process
	err := ri.Runner.Run(ctx, "pgrep", "-f", "Runner.Listener")
	if err == nil {
		return "running (process)", nil
	}

	return "installed, not running", nil
}

// Uninstall removes the runner agent and optionally deletes the install directory.
func (ri *RunnerInstaller) Uninstall(ctx context.Context, deleteDir bool) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("runner uninstall is only supported on Linux")
	}

	dir := expandHome(ri.InstallDir)

	// Stop and uninstall systemd service if present (best-effort)
	svcScript := filepath.Join(dir, "svc.sh")
	if _, err := os.Stat(svcScript); err == nil {
		fmt.Println("Stopping runner service...")
		_ = ri.Runner.RunInDir(ctx, dir, "sudo", svcScript, "stop")
		_ = ri.Runner.RunInDir(ctx, dir, "sudo", svcScript, "uninstall")
	}

	// Get removal token
	fmt.Println("Obtaining removal token...")
	tokenBytes, err := ri.Runner.RunOutput(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/actions/runners/remove-token", ri.Repo),
		"--method", "POST", "--jq", ".token")
	if err != nil {
		return fmt.Errorf("getting removal token: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))

	// Remove runner configuration
	fmt.Println("Removing runner configuration...")
	configScript := filepath.Join(dir, "config.sh")
	if err := ri.Runner.RunInDir(ctx, dir, configScript, "remove", "--token", token); err != nil {
		return fmt.Errorf("removing runner: %w", err)
	}

	if deleteDir {
		fmt.Printf("Deleting install directory %s\n", dir)
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("deleting install directory: %w", err)
		}
	}

	fmt.Println("Runner uninstalled.")
	return nil
}

// ParseRepoFromRemote extracts "owner/repo" from a git remote URL.
// Supports both SSH (git@github.com:owner/repo.git) and HTTPS (https://github.com/owner/repo.git).
func ParseRepoFromRemote(remoteURL string) (string, error) {
	remoteURL = strings.TrimSpace(remoteURL)

	// SSH: git@github.com:owner/repo.git
	sshRe := regexp.MustCompile(`git@github\.com:([^/]+/[^/]+?)(?:\.git)?$`)
	if m := sshRe.FindStringSubmatch(remoteURL); len(m) == 2 {
		return m[1], nil
	}

	// HTTPS: https://github.com/owner/repo.git
	httpsRe := regexp.MustCompile(`https://github\.com/([^/]+/[^/]+?)(?:\.git)?$`)
	if m := httpsRe.FindStringSubmatch(remoteURL); len(m) == 2 {
		return m[1], nil
	}

	return "", fmt.Errorf("cannot parse GitHub repo from remote URL: %s", remoteURL)
}

// expandHome replaces a leading ~/ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
