package wsl

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// ToWSLPath converts a Windows path to a WSL /mnt/ path.
// Examples: "F:\Source Code\UE" → "/mnt/f/Source Code/UE"
//
// If the path is already a Unix-style path (starts with /), it is returned unchanged.
// UNC paths (\\server\share) are not supported and return an error-prefixed string.
func ToWSLPath(windowsPath string) string {
	if windowsPath == "" {
		return ""
	}

	// Already a WSL/Unix path — pass through.
	if strings.HasPrefix(windowsPath, "/") {
		return windowsPath
	}

	// Normalize to forward slashes first.
	p := filepath.ToSlash(windowsPath)

	// UNC paths not supported.
	if rest, ok := strings.CutPrefix(p, "//"); ok {
		return fmt.Sprintf("/mnt/UNSUPPORTED_UNC/%s", rest)
	}

	// Expect "X:/..." or "X:..." — extract drive letter.
	if len(p) >= 2 && p[1] == ':' && unicode.IsLetter(rune(p[0])) {
		drive := strings.ToLower(string(p[0]))
		rest := strings.TrimPrefix(p[2:], "/")
		if rest == "" {
			return "/mnt/" + drive
		}
		return "/mnt/" + drive + "/" + rest
	}

	// Relative or unusual path — prefix with /mnt/ as best effort.
	return p
}

// ToWindowsPath converts a WSL /mnt/<drive>/... path back to a Windows path.
// Examples: "/mnt/f/Source Code/UE" → "F:\Source Code\UE"
//
// Non-/mnt/ paths (native WSL paths like ~/ludus/engine/) are returned unchanged.
func ToWindowsPath(wslPath string) string {
	if wslPath == "" {
		return ""
	}

	const prefix = "/mnt/"
	if !strings.HasPrefix(wslPath, prefix) {
		return wslPath
	}

	rest := strings.TrimPrefix(wslPath, prefix)
	if rest == "" {
		return wslPath
	}

	// Extract drive letter.
	drive := strings.ToUpper(string(rest[0]))
	remainder := rest[1:]
	if remainder == "" {
		return drive + `:\`
	}
	remainder = strings.TrimPrefix(remainder, "/")
	return drive + `:\` + filepath.FromSlash(remainder)
}

// IsNativePath returns true if the path is a native WSL2 ext4 path
// (i.e., not under /mnt/ which is virtiofs-backed).
func IsNativePath(path string) bool {
	if path == "" {
		return false
	}
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "/mnt/")
}
