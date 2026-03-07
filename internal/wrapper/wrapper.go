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

// BinaryPath returns the path to the cached wrapper binary for the given architecture.
// arch should be "amd64" or "arm64".
func BinaryPath(cacheDir, arch string) string {
	if arch == "" {
		arch = "amd64"
	}
	return filepath.Join(cacheDir, "out", "linux", arch,
		"gamelift-servers-managed-containers", "amazon-gamelift-servers-game-server-wrapper")
}

// EnsureBinary clones and builds the Amazon GameLift Game Server Wrapper,
// returning the path to the built binary. Results are cached in ~/.cache/ludus/.
// arch should be "amd64" or "arm64" (defaults to "amd64").
func EnsureBinary(ctx context.Context, r *runner.Runner, arch string) (string, error) {
	if arch == "" {
		arch = "amd64"
	}

	cacheDir, err := CacheDir()
	if err != nil {
		return "", err
	}

	binaryPath := BinaryPath(cacheDir, arch)

	// Check if cached binary already exists
	if _, err := os.Stat(binaryPath); err == nil {
		fmt.Printf("  Using cached game server wrapper binary (%s)\n", arch)
		return binaryPath, nil
	}

	// Clone the repository if not already present
	srcDir := filepath.Join(cacheDir, "src")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		fmt.Println("  Cloning game server wrapper repository...")
		if err := os.MkdirAll(filepath.Dir(cacheDir), 0755); err != nil {
			return "", fmt.Errorf("creating cache directory: %w", err)
		}
		// Remove stale cache if it exists but source is missing
		os.RemoveAll(cacheDir)

		if err := r.Run(ctx, "git", "clone", "--branch", WrapperVersion, "--depth", "1",
			WrapperRepo, cacheDir); err != nil {
			return "", fmt.Errorf("cloning game server wrapper: %w", err)
		}
	}

	// Build the wrapper
	fmt.Printf("  Building game server wrapper for %s...\n", arch)
	if err := buildWrapper(ctx, r, cacheDir, arch); err != nil {
		return "", fmt.Errorf("building game server wrapper: %w", err)
	}

	// Verify the binary was produced
	if _, err := os.Stat(binaryPath); err != nil {
		return "", fmt.Errorf("wrapper binary not found after build at %s", binaryPath)
	}

	return binaryPath, nil
}

// buildWrapper builds the game server wrapper binary. On systems with make,
// it delegates to `make build` for amd64. On Windows (where make is typically
// absent) or for arm64, it runs the equivalent steps directly.
func buildWrapper(ctx context.Context, r *runner.Runner, cacheDir, arch string) error {
	if runtime.GOOS != "windows" && arch == "amd64" {
		return r.RunInDir(ctx, cacheDir, "make", "build")
	}
	return buildWrapperWindows(ctx, r, cacheDir, arch)
}

// buildWrapperWindows replicates the Makefile's `build` target on Windows
// (or for non-amd64 architectures) using curl and Go cross-compilation.
func buildWrapperWindows(ctx context.Context, r *runner.Runner, cacheDir, arch string) error {
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
		if runtime.GOOS == "windows" {
			if err := r.Run(ctx, "powershell", "-NoProfile", "-Command",
				fmt.Sprintf("Expand-Archive -Path '%s' -DestinationPath '%s' -Force", sdkZip, sdkDir)); err != nil {
				return fmt.Errorf("extracting server SDK: %w", err)
			}
		} else {
			if err := r.Run(ctx, "unzip", "-o", sdkZip, "-d", sdkDir); err != nil {
				return fmt.Errorf("extracting server SDK: %w", err)
			}
		}
	}

	// Cross-compile for linux/<arch>
	srcDir := filepath.Join(cacheDir, "src")
	outDir := filepath.Join(cacheDir, "out", "linux", arch)
	binaryDir := filepath.Join(outDir, "gamelift-servers-managed-containers")
	if err := os.MkdirAll(binaryDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	binaryPath := filepath.Join(binaryDir, "amazon-gamelift-servers-game-server-wrapper")
	ldflags := fmt.Sprintf("-X '%s/internal.version=1.1.0'", appPackage)

	buildRunner := *r
	buildRunner.Env = append(buildRunner.Env, "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
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
