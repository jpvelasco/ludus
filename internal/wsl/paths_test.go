package wsl

import "testing"

// TestToWSLPath verifies Windows→WSL path conversion.
// These tests must pass on Windows, Linux, and macOS: the implementation uses
// strings.ReplaceAll (not filepath.ToSlash, which is a no-op on Linux/macOS)
// so backslash normalization works regardless of the host OS.
func TestToWSLPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		// Windows backslash paths — must work on all host OSes.
		{"simple drive", `C:\Users\jp`, "/mnt/c/Users/jp"},
		{"lowercase drive", `f:\Source`, "/mnt/f/Source"},
		{"uppercase drive", `F:\Source`, "/mnt/f/Source"},
		{"path with spaces", `F:\Source Code\UnrealEngine`, "/mnt/f/Source Code/UnrealEngine"},
		{"drive root", `C:\`, "/mnt/c"},
		{"deep nested", `D:\a\b\c\d\e`, "/mnt/d/a/b/c/d/e"},
		// Forward-slash Windows paths (e.g. from Go filepath on Windows or user input).
		{"drive with forward slashes", "C:/Users/jp", "/mnt/c/Users/jp"},
		{"drive letter only", "C:", "/mnt/c"},
		// Already a Unix/WSL path — pass through unchanged.
		{"already unix path", "/mnt/f/Source", "/mnt/f/Source"},
		{"native wsl path", "/home/user/ludus", "/home/user/ludus"},
		{"unc path", `\\server\share\file`, "/mnt/UNSUPPORTED_UNC/server/share/file"},
		{"relative path passthrough", "relative/path", "relative/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToWSLPath(tt.input)
			if got != tt.want {
				t.Errorf("ToWSLPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestToWindowsPath verifies WSL→Windows path conversion.
// The implementation uses strings.ReplaceAll so forward slashes are replaced
// with backslashes on all host OSes (filepath.FromSlash is a no-op on Linux/macOS).
func TestToWindowsPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"simple mount", "/mnt/c/Users/jp", `C:\Users\jp`},
		{"lowercase drive", "/mnt/f/Source Code/UE", `F:\Source Code\UE`},
		{"drive root", "/mnt/c", `C:\`},
		{"deep path", "/mnt/d/a/b/c", `D:\a\b\c`},
		// Non-/mnt/ paths pass through unchanged on all platforms.
		{"native wsl path passthrough", "/home/user/ludus", "/home/user/ludus"},
		{"bare mnt", "/mnt/", "/mnt/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToWindowsPath(tt.input)
			if got != tt.want {
				t.Errorf("ToWindowsPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple path", "/opt/ue/5.7", "'/opt/ue/5.7'"},
		{"path with spaces", "/mnt/f/Source Code/UE", "'/mnt/f/Source Code/UE'"},
		{"native path", "/home/user/ludus/engine", "'/home/user/ludus/engine'"},
		{"empty", "", "''"},
		{"single quote in path", "/opt/it's/ue", `'/opt/it'\''s/ue'`},
		{"ddc key=value", "UE-LocalDataCachePath=/home/user/ddc", "'UE-LocalDataCachePath=/home/user/ddc'"},
		{"ddc value with spaces", "UE-LocalDataCachePath=/home/user/my ddc", "'UE-LocalDataCachePath=/home/user/my ddc'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuote(tt.input)
			if got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsNativePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"native home", "/home/user", true},
		{"native tmp", "/tmp/foo", true},
		{"mnt path", "/mnt/c/Users", false},
		{"mnt root", "/mnt/", false},
		{"windows path", `C:\Users`, false},
		{"root", "/", true},
		{"native ludus", "/home/user/ludus/engine", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNativePath(tt.input)
			if got != tt.want {
				t.Errorf("IsNativePath(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
