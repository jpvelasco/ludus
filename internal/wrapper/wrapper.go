package wrapper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/devrecon/ludus/internal/runner"
)

const (
	// WrapperRepo is the Git repository for the Amazon GameLift Game Server Wrapper.
	WrapperRepo = "https://github.com/amazon-gamelift/amazon-gamelift-servers-game-server-wrapper.git"
	// WrapperVersion is the Git tag to clone.
	WrapperVersion = "v1.1.0"
	// serverSDKURL is the GameLift Server SDK download URL used by the Makefile.
	serverSDKURL = "https://github.com/amazon-gamelift/amazon-gamelift-servers-go-server-sdk/releases/download/v5.4.0/GameLift-Go-ServerSDK-5.4.0.zip"
	// appPackage is the Go module path for ldflags version injection.
	appPackage = "github.com/amazon-gamelift/amazon-gamelift-servers-game-server-wrapper"
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
	if err := buildWrapper(ctx, r, cacheDir); err != nil {
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

// buildWrapper builds the game server wrapper binary. On systems with make,
// it delegates to `make build`. On Windows (where make is typically absent),
// it runs the equivalent steps directly: download the server SDK, cross-compile
// the Go binary for linux/amd64, and copy the config file.
func buildWrapper(ctx context.Context, r *runner.Runner, cacheDir string) error {
	if runtime.GOOS != "windows" {
		return r.RunInDir(ctx, cacheDir, "make", "build")
	}
	return buildWrapperWindows(ctx, r, cacheDir)
}

// buildWrapperWindows replicates the Makefile's `build` target on Windows
// without requiring make, using curl and Go cross-compilation.
func buildWrapperWindows(ctx context.Context, r *runner.Runner, cacheDir string) error {
	sdkZip := filepath.Join(cacheDir, "gamelift-servers-server-sdk.zip")
	sdkDir := filepath.Join(cacheDir, "src", "ext", "gamelift-servers-server-sdk")

	// Download the GameLift Server SDK if not already present
	if _, err := os.Stat(sdkZip); err != nil {
		fmt.Println("  Downloading GameLift Server SDK...")
		if err := r.RunInDir(ctx, cacheDir, "curl", "-L", serverSDKURL, "-o", sdkZip); err != nil {
			return fmt.Errorf("downloading server SDK: %w", err)
		}
	}

	// Extract the SDK
	if _, err := os.Stat(sdkDir); err != nil {
		if err := os.MkdirAll(sdkDir, 0755); err != nil {
			return fmt.Errorf("creating SDK directory: %w", err)
		}
		// Use PowerShell to extract since unzip may not be available
		if err := r.Run(ctx, "powershell", "-NoProfile", "-Command",
			fmt.Sprintf("Expand-Archive -Path '%s' -DestinationPath '%s' -Force", sdkZip, sdkDir)); err != nil {
			return fmt.Errorf("extracting server SDK: %w", err)
		}
	}

	// Cross-compile for linux/amd64
	srcDir := filepath.Join(cacheDir, "src")
	outDir := filepath.Join(cacheDir, "out", "linux", "amd64")
	binaryDir := filepath.Join(outDir, "gamelift-servers-managed-containers")
	if err := os.MkdirAll(binaryDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	binaryPath := filepath.Join(binaryDir, "amazon-gamelift-servers-game-server-wrapper")
	ldflags := fmt.Sprintf("-X '%s/internal.version=1.1.0'", appPackage)

	buildRunner := *r
	buildRunner.Env = append(buildRunner.Env, "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	if err := buildRunner.RunInDir(ctx, srcDir, "go", "build",
		"-trimpath", "-v",
		"-ldflags="+ldflags,
		"-o", binaryPath,
		"."); err != nil {
		return fmt.Errorf("go build: %w", err)
	}

	// Copy the managed-containers config template
	configSrc := filepath.Join(srcDir, "template", "template-managed-containers-config.yaml")
	configDst := filepath.Join(binaryDir, "config.yaml")
	configData, err := os.ReadFile(configSrc)
	if err != nil {
		return fmt.Errorf("reading config template: %w", err)
	}
	if err := os.WriteFile(configDst, configData, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}
