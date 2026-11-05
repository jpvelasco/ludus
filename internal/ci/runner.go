package ci

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/jpvelasco/ludus/internal/runner"
)

const (
	defaultRunnerVersion = "v2.338.2"
	// defaultRunnerSHA256 is the repository-pinned digest for the default
	// actions/runner release asset.
	defaultRunnerSHA256 = "baebf204307bac6193480852732f0aba2ae97b6c5597c244f4d5558390147c40"
)

var (
	// Keep version keys literal so changing defaultRunnerVersion without adding
	// its reviewed digest fails closed instead of reusing an old checksum.
	runnerSHA256ByVersion = map[string]string{
		"v2.338.2": defaultRunnerSHA256,
	}
	runnerVersionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)
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
	// Version is the runner release tag, a version without the "v" prefix, or
	// "latest". Empty uses the pinned default.
	Version string
	// AllowUnpinnedChecksum permits using the GitHub release API digest for a
	// version without a repository-pinned checksum, after printing a warning.
	AllowUnpinnedChecksum bool
	// Service controls whether to install as a systemd service.
	Service bool
}

// Install downloads and configures the GitHub Actions runner agent.
func (ri *RunnerInstaller) Install(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("runner install is only supported on Linux")
	}

	dir := expandHome(ri.InstallDir)
	token, version, err := ri.installMetadata(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Installing runner %s to %s\n", version, dir)
	if err := ri.installRunnerFiles(ctx, dir, version); err != nil {
		return err
	}
	if err := ri.configureRunner(ctx, dir, token); err != nil {
		return err
	}

	return ri.finishInstall(ctx, dir)
}

func (ri *RunnerInstaller) installMetadata(ctx context.Context) (token, version string, err error) {
	version, err = ri.resolveRunnerVersion(ctx)
	if err != nil {
		return "", "", err
	}
	if err := ri.validateRunnerChecksumPolicy(version); err != nil {
		return "", "", err
	}

	token, err = ri.registrationToken(ctx)
	if err != nil {
		return "", "", err
	}
	return token, version, nil
}

func (ri *RunnerInstaller) resolveRunnerVersion(ctx context.Context) (string, error) {
	requested := strings.TrimSpace(ri.Version)
	if requested == "" {
		return defaultRunnerVersion, nil
	}
	if strings.EqualFold(requested, "latest") {
		version, err := ri.latestRunnerVersion(ctx)
		if err != nil {
			return "", err
		}
		if ri.Runner.DryRun && version == "(dry-run)" {
			return defaultRunnerVersion, nil
		}
		return normalizeRunnerVersion(version)
	}
	return normalizeRunnerVersion(requested)
}

func (ri *RunnerInstaller) installRunnerFiles(ctx context.Context, dir, version string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating install directory: %w", err)
	}

	version, err := normalizeRunnerVersion(version)
	if err != nil {
		return err
	}
	checksum, err := ri.runnerSHA256(ctx, version)
	if err != nil {
		return err
	}
	tarball, err := ri.downloadRunner(ctx, dir, version)
	if err != nil {
		return err
	}
	if err := ri.verifyRunnerChecksum(ctx, dir, tarball, checksum); err != nil {
		return err
	}
	if err := ri.extractRunner(ctx, dir, tarball); err != nil {
		return err
	}
	return nil
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

func (ri *RunnerInstaller) runnerSHA256(ctx context.Context, version string) (string, error) {
	if digest, ok := runnerSHA256ByVersion[version]; ok {
		return digest, nil
	}
	if err := ri.validateRunnerChecksumPolicy(version); err != nil {
		return "", err
	}

	warningWriter := ri.Runner.Stderr
	if warningWriter == nil {
		warningWriter = os.Stderr
	}
	fmt.Fprintf(warningWriter, "WARNING: runner %s uses a SHA-256 digest from GitHub's release API, "+
		"the same source as the download. This detects corruption but does not independently authenticate the asset.\n",
		version)
	tarball := runnerArchiveName(version)
	release := fmt.Sprintf("repos/actions/runner/releases/tags/%s", version)
	jq := fmt.Sprintf(`.assets[] | select(.name == "%s") | .digest`, tarball)
	digestBytes, err := ri.Runner.RunOutput(ctx, "gh", "api", release, "--jq", jq)
	if err != nil {
		return "", fmt.Errorf("getting runner SHA-256 for %s: %w", version, err)
	}
	if ri.Runner.DryRun {
		return strings.Repeat("0", 64), nil
	}

	digest := strings.TrimPrefix(strings.TrimSpace(string(digestBytes)), "sha256:")
	if _, err := hex.DecodeString(digest); err != nil || len(digest) != 64 {
		return "", fmt.Errorf("GitHub release %s has no valid SHA-256 digest for %s", version, tarball)
	}
	return digest, nil
}

func (ri *RunnerInstaller) validateRunnerChecksumPolicy(version string) error {
	if _, ok := runnerSHA256ByVersion[version]; ok || ri.AllowUnpinnedChecksum {
		return nil
	}
	return fmt.Errorf("runner version %s has no repository-pinned SHA-256; add it to the pinned checksum table "+
		"or pass --allow-unpinned-checksum to accept the release API digest", version)
}

func (ri *RunnerInstaller) downloadRunner(ctx context.Context, dir, version string) (string, error) {
	version, err := normalizeRunnerVersion(version)
	if err != nil {
		return "", err
	}
	tarball := runnerArchiveName(version)
	tarballPath := filepath.Join(dir, tarball)
	downloadURL := fmt.Sprintf("https://github.com/actions/runner/releases/download/%s/%s", version, tarball)

	fmt.Println("Downloading runner...")
	if err := ri.Runner.Run(ctx, "curl", "-fsSL", "-o", tarballPath, downloadURL); err != nil {
		_ = os.Remove(tarballPath)
		return "", fmt.Errorf("downloading runner: %w", err)
	}
	return tarball, nil
}

func (ri *RunnerInstaller) verifyRunnerChecksum(ctx context.Context, dir, tarball, checksum string) error {
	tarballPath := filepath.Join(dir, tarball)
	digestBytes, err := ri.Runner.RunOutput(ctx, "sha256sum", tarballPath)
	if err != nil {
		_ = os.Remove(tarballPath)
		return fmt.Errorf("verifying runner checksum: %w", err)
	}
	if ri.Runner.DryRun {
		return nil
	}

	digestFields := strings.Fields(string(digestBytes))
	if len(digestFields) == 0 || digestFields[0] != checksum {
		_ = os.Remove(tarballPath)
		return fmt.Errorf("verifying runner checksum: SHA-256 mismatch for %s", tarball)
	}
	return nil
}

func runnerArchiveName(version string) string {
	return fmt.Sprintf("actions-runner-linux-x64-%s.tar.gz", strings.TrimPrefix(version, "v"))
}

func normalizeRunnerVersion(version string) (string, error) {
	version = strings.TrimSpace(version)
	if !runnerVersionPattern.MatchString(version) {
		return "", fmt.Errorf("invalid runner version %q: expected a version such as v2.338.2", version)
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version, nil
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
		// A dry-run uninstall previews the deletion like the runner previews
		// the stop/remove commands above; the installation must survive (#707).
		if ri.Runner.DryRun {
			fmt.Printf("Dry run: would delete install directory %s\n", dir)
		} else {
			fmt.Printf("Deleting install directory %s\n", dir)
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("deleting install directory: %w", err)
			}
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
