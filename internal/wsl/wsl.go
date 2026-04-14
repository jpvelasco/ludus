// Package wsl provides a WSL2 build backend for compiling Unreal Engine
// and game servers directly inside a WSL2 distro, bypassing container runtimes.
//
// Two modes:
//   - Default (--backend wsl2): accesses source via /mnt/<drive>/ (virtiofs).
//     Zero setup, but I/O-bound on large codebases (~50 MB/s for small files).
//   - Native (--backend wsl2 --wsl-native): rsyncs source to ~/ludus/engine/<ver>/
//     on the distro's native ext4 filesystem. DDC cache also lives on ext4 at
//     ~/ludus/ddc/. One-time sync cost, 3-10x faster I/O for builds and cooking.
package wsl

import (
	"context"
	"fmt"
	"strings"

	"github.com/devrecon/ludus/internal/runner"
)

// WSL2 is the coordinator for WSL2 build operations.
type WSL2 struct {
	Info   *Info
	Distro string
	Runner *runner.Runner
}

// New detects WSL2 availability, picks a distro, and returns a ready coordinator.
// Pass distroOverride to select a specific distro instead of the default.
func New(r *runner.Runner, distroOverride string) (*WSL2, error) {
	info, err := Detect()
	if err != nil {
		return nil, fmt.Errorf("detecting WSL2: %w", err)
	}
	if !info.Available {
		return nil, fmt.Errorf("WSL2 is not available; install with: wsl --install")
	}

	distro, err := PickDistro(info, distroOverride)
	if err != nil {
		return nil, err
	}

	return &WSL2{
		Info:   info,
		Distro: distro,
		Runner: r,
	}, nil
}

// ToWSLPath converts a Windows path to a WSL mount path.
func (w *WSL2) ToWSLPath(windowsPath string) string {
	return ToWSLPath(windowsPath)
}

// Run executes a command inside the WSL2 distro.
func (w *WSL2) Run(ctx context.Context, args ...string) error {
	return Run(ctx, w.Runner, w.Distro, args...)
}

// RunBash executes a bash script string inside the WSL2 distro.
func (w *WSL2) RunBash(ctx context.Context, script string) error {
	return RunBash(ctx, w.Runner, w.Distro, script)
}

// RunOutput executes a command and returns stdout.
func (w *WSL2) RunOutput(ctx context.Context, args ...string) ([]byte, error) {
	return RunOutput(ctx, w.Runner, w.Distro, args...)
}

// EnsureDeps checks and optionally installs build dependencies.
func (w *WSL2) EnsureDeps(ctx context.Context) error {
	if err := CheckDeps(ctx, w.Runner, w.Distro); err != nil {
		fmt.Printf("Installing build dependencies in WSL2 distro %q...\n", w.Distro)
		return InstallDeps(ctx, w.Runner, w.Distro)
	}
	return nil
}

// EnsureRuntimeDeps checks and installs runtime libraries needed by
// UnrealEditor-Cmd for cooking. Idempotent: skips install if libnss3 is found.
func (w *WSL2) EnsureRuntimeDeps(ctx context.Context) error {
	if err := CheckRuntimeDeps(ctx, w.Runner, w.Distro); err != nil {
		fmt.Printf("Installing runtime dependencies in WSL2 distro %q...\n", w.Distro)
		return InstallRuntimeDeps(ctx, w.Runner, w.Distro)
	}
	return nil
}

// DiskFreeGB returns the free disk space in GB on the distro's root filesystem.
func (w *WSL2) DiskFreeGB(ctx context.Context) (float64, error) {
	return CheckDiskSpace(ctx, w.Runner, w.Distro)
}

// ExpandHomePaths replaces the literal string "$HOME" in each of the provided
// paths with the distro's actual home directory. Paths that do not contain
// "$HOME" are returned unchanged. A single WSL2 round-trip is made only when
// at least one path requires expansion.
func (w *WSL2) ExpandHomePaths(ctx context.Context, paths ...string) ([]string, error) {
	needsExpand := false
	for _, p := range paths {
		if strings.Contains(p, "$HOME") {
			needsExpand = true
			break
		}
	}
	if !needsExpand {
		return paths, nil
	}
	out, err := w.RunOutput(ctx, "bash", "-c", "echo $HOME")
	if err != nil {
		return nil, fmt.Errorf("resolving WSL2 home directory: %w", err)
	}
	home := strings.TrimSpace(string(out))
	if home == "" {
		return nil, fmt.Errorf("WSL2 home directory resolved to empty string")
	}
	result := make([]string, len(paths))
	for i, p := range paths {
		result[i] = strings.ReplaceAll(p, "$HOME", home)
	}
	return result, nil
}

// HasRsync checks if rsync is available in the distro.
func (w *WSL2) HasRsync(ctx context.Context) bool {
	return CheckRsync(ctx, w.Runner, w.Distro)
}
