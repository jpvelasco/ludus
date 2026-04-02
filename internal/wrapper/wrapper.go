package wrapper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/devrecon/ludus/internal/retry"
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

// BinaryPath returns the path to the cached wrapper binary for the given OS and architecture.
// targetOS should be "linux" or "windows"; arch should be "amd64" or "arm64".
func BinaryPath(cacheDir, targetOS, arch string) string {
	if targetOS == "" {
		targetOS = "linux"
	}
	if arch == "" {
		arch = "amd64"
	}
	name := "amazon-gamelift-servers-game-server-wrapper"
	if targetOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(cacheDir, "out", targetOS, arch,
		"gamelift-servers-managed-containers", name)
}

// EnsureBinary clones and builds the Amazon GameLift Game Server Wrapper,
// returning the path to the built binary. Results are cached in ~/.cache/ludus/.
// targetOS should be "linux" or "windows" (defaults to "linux").
// arch should be "amd64" or "arm64" (defaults to "amd64").
func EnsureBinary(ctx context.Context, r *runner.Runner, targetOS, arch string) (string, error) {
	if targetOS == "" {
		targetOS = "linux"
	}
	if arch == "" {
		arch = "amd64"
	}

	cacheDir, err := CacheDir()
	if err != nil {
		return "", err
	}

	binaryPath := BinaryPath(cacheDir, targetOS, arch)

	// Check if cached binary already exists
	if _, err := os.Stat(binaryPath); err == nil {
		fmt.Printf("  Using cached game server wrapper binary (%s/%s)\n", targetOS, arch)
		return binaryPath, nil
	}

	if err := ensureSource(ctx, r, cacheDir); err != nil {
		return "", err
	}

	// Build the wrapper
	fmt.Printf("  Building game server wrapper for %s/%s...\n", targetOS, arch)
	if err := buildWrapper(ctx, r, cacheDir, targetOS, arch); err != nil {
		return "", fmt.Errorf("building game server wrapper: %w", err)
	}

	// Verify the binary was produced
	if _, err := os.Stat(binaryPath); err != nil {
		return "", fmt.Errorf("wrapper binary not found after build at %s", binaryPath)
	}

	return binaryPath, nil
}

// ensureSource clones the game server wrapper repository if not already present.
func ensureSource(ctx context.Context, r *runner.Runner, cacheDir string) error {
	srcDir := filepath.Join(cacheDir, "src")
	if _, err := os.Stat(srcDir); !os.IsNotExist(err) {
		return nil
	}
	fmt.Println("  Cloning game server wrapper repository...")
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}
	if err := retry.Do(ctx, retry.Default(), func() error {
		os.RemoveAll(cacheDir)
		return r.Run(ctx, "git", "clone", "--branch", WrapperVersion, "--depth", "1",
			WrapperRepo, cacheDir)
	}); err != nil {
		return fmt.Errorf("cloning game server wrapper: %w", err)
	}
	return nil
}

// buildWrapper builds the game server wrapper binary. On systems with make,
// it delegates to `make build` for native linux/amd64. Otherwise it runs
// the equivalent steps directly (cross-compilation or non-Linux targets).
func buildWrapper(ctx context.Context, r *runner.Runner, cacheDir, targetOS, arch string) error {
	if runtime.GOOS == "linux" && targetOS == "linux" && arch == "amd64" {
		return r.RunInDir(ctx, cacheDir, "make", "build")
	}
	return buildWrapperCross(ctx, r, cacheDir, targetOS, arch)
}

// buildWrapperCross replicates the Makefile's build target using curl and
// Go cross-compilation. Used for non-Linux-amd64 targets or Windows hosts.
func buildWrapperCross(ctx context.Context, r *runner.Runner, cacheDir, targetOS, arch string) error {
	if err := downloadWrapperSource(ctx, r, cacheDir); err != nil {
		return err
	}
	return buildWrapperBinary(ctx, r, cacheDir, targetOS, arch)
}

// downloadWrapperSource downloads and extracts the GameLift Server SDK
// into the cache directory if not already present.
func downloadWrapperSource(ctx context.Context, r *runner.Runner, cacheDir string) error {
	sdkZip := filepath.Join(cacheDir, "gamelift-servers-server-sdk.zip")
	sdkDir := filepath.Join(cacheDir, "src", "ext", "gamelift-servers-server-sdk")

	// Download the GameLift Server SDK if not already present
	if _, err := os.Stat(sdkZip); err != nil {
		fmt.Println("  Downloading GameLift Server SDK...")
		if err := retry.Do(ctx, retry.Default(), func() error {
			return r.RunInDir(ctx, cacheDir, "curl", "-L", serverSDKURL, "-o", sdkZip)
		}); err != nil {
			return fmt.Errorf("downloading server SDK: %w", err)
		}
	}

	// Extract the SDK if not already present
	if _, err := os.Stat(sdkDir); err != nil {
		if err := os.MkdirAll(sdkDir, 0755); err != nil {
			return fmt.Errorf("creating SDK directory: %w", err)
		}
		if err := extractSDK(ctx, r, sdkZip, sdkDir); err != nil {
			return err
		}
	}

	return nil
}

// extractSDK extracts the SDK zip using the platform-appropriate tool.
func extractSDK(ctx context.Context, r *runner.Runner, sdkZip, sdkDir string) error {
	if runtime.GOOS == "windows" {
		if err := r.Run(ctx, "powershell", "-NoProfile", "-Command",
			fmt.Sprintf("Expand-Archive -Path '%s' -DestinationPath '%s' -Force", sdkZip, sdkDir)); err != nil {
			return fmt.Errorf("extracting server SDK: %w", err)
		}
		return nil
	}
	if err := r.Run(ctx, "unzip", "-o", sdkZip, "-d", sdkDir); err != nil {
		return fmt.Errorf("extracting server SDK: %w", err)
	}
	return nil
}

// buildWrapperBinary cross-compiles the wrapper for targetOS/arch and copies
// the config template into the output directory.
func buildWrapperBinary(ctx context.Context, r *runner.Runner, cacheDir, targetOS, arch string) error {
	srcDir := filepath.Join(cacheDir, "src")
	binaryPath := BinaryPath(cacheDir, targetOS, arch)
	binaryDir := filepath.Dir(binaryPath)
	if err := os.MkdirAll(binaryDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	ldflags := fmt.Sprintf("-X '%s/internal.version=1.1.0'", appPackage)

	buildRunner := *r
	buildRunner.Env = append(slices.Clone(r.Env), "CGO_ENABLED=0", "GOOS="+targetOS, "GOARCH="+arch)
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
