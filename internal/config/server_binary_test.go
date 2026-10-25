package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalServerBinaryName(t *testing.T) {
	tests := []struct {
		name         string
		serverTarget string
		binPlatform  string
		serverConfig string
		want         string
	}{
		{"Shipping suffixes the target", "ExampleServer", "Linux", "Shipping", "ExampleServer-Linux-Shipping"},
		{"Test suffixes the target", "ExampleServer", "Linux", "Test", "ExampleServer-Linux-Test"},
		{"arm64 platform segment", "LyraServer", "LinuxArm64", "Shipping", "LyraServer-LinuxArm64-Shipping"},
		{"Development keeps bare target", "ExampleServer", "Linux", "Development", "ExampleServer"},
		{"empty config keeps bare target", "ExampleServer", "Linux", "", "ExampleServer"},
		{"unknown config keeps bare target", "ExampleServer", "Linux", "DebugGame", "ExampleServer"},
		{"config is case-insensitive and trimmed", "ExampleServer", "Linux", " shipping ", "ExampleServer-Linux-shipping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanonicalServerBinaryName(tt.serverTarget, tt.binPlatform, tt.serverConfig)
			if got != tt.want {
				t.Errorf("CanonicalServerBinaryName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// stageServerBinaries creates files inside the packaged Binaries directory for
// the given build layout and returns that directory.
func stageServerBinaries(t *testing.T, serverBuildDir, packagedDirName, binPlatform string, filenames []string) string {
	t.Helper()
	binDir := serverBinariesDir(serverBuildDir, packagedDirName, binPlatform)
	for _, name := range filenames {
		path := filepath.Join(binDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return binDir
}

func TestDiscoverServerBinaryName(t *testing.T) {
	tests := []struct {
		name         string
		arch         string
		staged       []string
		serverConfig string
		wantName     string
	}{
		// #718: discovery must include the Shipping/Test suffix and search the
		// packaged directory (uproject basename), not the bare target.
		{name: "suffixed Shipping executable", arch: "amd64", staged: []string{"ExampleServer-Linux-Shipping"}, wantName: "ExampleServer-Linux-Shipping"},
		{name: "bare Development executable", arch: "amd64", staged: []string{"ExampleServer"}, wantName: "ExampleServer"},
		{name: "suffixed wins when both staged", arch: "amd64", staged: []string{"ExampleServer", "ExampleServer-Linux-Shipping"}, wantName: "ExampleServer-Linux-Shipping"},
		{name: "multi-dot sidecars skipped", arch: "amd64", staged: []string{"ExampleServer-Linux-Test.target", "ExampleServer-Linux-Test"}, wantName: "ExampleServer-Linux-Test"},
		{name: "arm64 binaries platform", arch: "arm64", staged: []string{"ExampleServer-LinuxArm64-Shipping"}, serverConfig: "Shipping", wantName: "ExampleServer-LinuxArm64-Shipping"},
		{name: "canonical Shipping fallback when nothing staged", arch: "amd64", staged: nil, serverConfig: "Shipping", wantName: "ExampleServer-Linux-Shipping"},
		{name: "bare fallback when nothing staged", arch: "amd64", staged: nil, wantName: "ExampleServer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverBuildDir := t.TempDir()
			stageServerBinaries(t, serverBuildDir, "ExampleContent", BinariesPlatformDir(tt.arch), tt.staged)
			got := DiscoverServerBinaryName(serverBuildDir, "ExampleContent", "ExampleServer", tt.arch, tt.serverConfig)
			if got != tt.wantName {
				t.Errorf("DiscoverServerBinaryName() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestDiscoverServerBinary(t *testing.T) {
	serverBuildDir := t.TempDir()
	tests := []struct {
		name         string
		arch         string
		staged       []string
		wantFilename string
	}{
		{name: "packaged dir differs from project name", arch: "amd64", staged: nil, wantFilename: "ExampleServer"},
		{name: "discovers suffixed executable", arch: "amd64", staged: []string{"ExampleServer-Linux-Shipping"}, wantFilename: "ExampleServer-Linux-Shipping"},
		{name: "arm64 path layout", arch: "arm64", staged: []string{"ExampleServer-LinuxArm64-Shipping"}, wantFilename: "ExampleServer-LinuxArm64-Shipping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := stageServerBinaries(t, serverBuildDir, "ExampleContent", BinariesPlatformDir(tt.arch), tt.staged)
			want := filepath.Join(binDir, tt.wantFilename)
			got := DiscoverServerBinary(serverBuildDir, "ExampleContent", "ExampleServer", tt.arch, "")
			if got != want {
				t.Errorf("DiscoverServerBinary() = %q, want %q", got, want)
			}
		})
	}
}
