package wrapper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/devrecon/ludus/internal/runner"
)

const (
	// WrapperRepo is the Git repository for the Amazon GameLift Game Server Wrapper.
	WrapperRepo = "https://github.com/amazon-gamelift/amazon-gamelift-servers-game-server-wrapper.git"
	// WrapperVersion is the Git tag to clone.
	WrapperVersion = "v1.1.0"
)

// CacheDir returns the cache directory for the game server wrapper.
func CacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "ludus", "game-server-wrapper"), nil
}

// BinaryPath returns the path to the cached wrapper binary.
func BinaryPath(cacheDir string) string {
	return filepath.Join(cacheDir, "out", "linux", "amd64",
		"gamelift-servers-managed-containers", "amazon-gamelift-servers-game-server-wrapper")
}

// EnsureBinary clones and builds the Amazon GameLift Game Server Wrapper,
// returning the path to the built binary. Results are cached in ~/.cache/ludus/.
func EnsureBinary(ctx context.Context, r *runner.Runner) (string, error) {
	cacheDir, err := CacheDir()
	if err != nil {
		return "", err
	}

	binaryPath := BinaryPath(cacheDir)

	// Check if cached binary already exists
	if _, err := os.Stat(binaryPath); err == nil {
		fmt.Println("  Using cached game server wrapper binary")
		return binaryPath, nil
	}

	// Clone the repository
	fmt.Println("  Cloning game server wrapper repository...")
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0755); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}
	// Remove stale cache if it exists but binary is missing
	os.RemoveAll(cacheDir)

	if err := r.Run(ctx, "git", "clone", "--branch", WrapperVersion, "--depth", "1",
		WrapperRepo, cacheDir); err != nil {
		return "", fmt.Errorf("cloning game server wrapper: %w", err)
	}

	// Build the wrapper
	fmt.Println("  Building game server wrapper...")
	if err := r.RunInDir(ctx, cacheDir, "make", "build"); err != nil {
		// Clean up on build failure so next run retries
		os.RemoveAll(cacheDir)
		return "", fmt.Errorf("building game server wrapper: %w", err)
	}

	// Verify the binary was produced
	if _, err := os.Stat(binaryPath); err != nil {
		os.RemoveAll(cacheDir)
		return "", fmt.Errorf("wrapper binary not found after build at %s", binaryPath)
	}

	return binaryPath, nil
}
