package ddc

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// resolveHome returns the user's home directory. It prefers os.UserHomeDir()
// ($HOME / %USERPROFILE%), but that fails when the environment is stripped of
// HOME — which happens under SSM Run Command, some CI runners, and bare service
// contexts. In that case it falls back to the os/user database, then to /root
// for the common root-in-container case, so DDC path resolution doesn't hard-fail.
func resolveHome() (string, error) {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home, nil
	}
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return u.HomeDir, nil
	}
	if runtime.GOOS != "windows" {
		// Last-resort default for HOME-less *nix contexts (e.g. SSM as root).
		return "/root", nil
	}
	return "", fmt.Errorf("resolving home directory: HOME is unset and no fallback is available; set HOME or configure ddc.zenPath/ddc.localPath explicitly")
}

// DefaultZenPath returns the default ZenStore host directory: ~/.ludus/zen
func DefaultZenPath() (string, error) {
	home, err := resolveHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ludus", "zen"), nil
}

// ResolveZenPath returns the override path if non-empty, otherwise DefaultZenPath.
func ResolveZenPath(override string) (string, error) {
	if override != "" {
		if !filepath.IsAbs(override) {
			return "", fmt.Errorf("DDC zen path must be absolute (got %q)", override)
		}
		return override, nil
	}
	return DefaultZenPath()
}

// DefaultPath returns the default DDC directory path: ~/.ludus/ddc
func DefaultPath() (string, error) {
	home, err := resolveHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ludus", "ddc"), nil
}

// ResolvePath returns the override path if non-empty, otherwise returns DefaultPath.
// Returns an error if the override is a relative path.
func ResolvePath(override string) (string, error) {
	if override != "" {
		if !filepath.IsAbs(override) {
			return "", fmt.Errorf("DDC path must be absolute (got %q); use a full path like /home/user/.ludus/ddc", override)
		}
		return override, nil
	}
	return DefaultPath()
}
