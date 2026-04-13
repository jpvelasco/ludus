package wsl

import "testing"

func TestToWSLPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"simple drive", `C:\Users\jp`, "/mnt/c/Users/jp"},
		{"drive with forward slashes", "C:/Users/jp", "/mnt/c/Users/jp"},
		{"lowercase drive", `f:\Source`, "/mnt/f/Source"},
		{"uppercase drive", `F:\Source`, "/mnt/f/Source"},
		{"path with spaces", `F:\Source Code\UnrealEngine`, "/mnt/f/Source Code/UnrealEngine"},
		{"drive root", `C:\`, "/mnt/c"},
		{"drive letter only", "C:", "/mnt/c"},
		{"already unix path", "/mnt/f/Source", "/mnt/f/Source"},
		{"native wsl path", "/home/user/ludus", "/home/user/ludus"},
		{"unc path", `\\server\share\file`, "/mnt/UNSUPPORTED_UNC/server/share/file"},
		{"relative path passthrough", "relative/path", "relative/path"},
		{"deep nested", `D:\a\b\c\d\e`, "/mnt/d/a/b/c/d/e"},
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
		{"native wsl path passthrough", "/home/user/ludus", "/home/user/ludus"},
		{"bare mnt", "/mnt/", "/mnt/"},
		{"deep path", "/mnt/d/a/b/c", `D:\a\b\c`},
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
