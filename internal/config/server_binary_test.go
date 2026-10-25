package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalServerBinaryName(t *testing.T) {
	tests := []struct {
		name             string
		serverTarget     string
		binPlatform      string
		serverConfig     string
		executableSuffix string
		want             string
	}{
		{"Shipping suffixes the target", "ExampleServer", "Linux", "Shipping", "", "ExampleServer-Linux-Shipping"},
		{"Test suffixes the target", "ExampleServer", "Linux", "Test", "", "ExampleServer-Linux-Test"},
		{"arm64 platform segment", "LyraServer", "LinuxArm64", "Shipping", "", "LyraServer-LinuxArm64-Shipping"},
		{"Development keeps bare target", "ExampleServer", "Linux", "Development", "", "ExampleServer"},
		{"empty config keeps bare target", "ExampleServer", "Linux", "", "", "ExampleServer"},
		{"unknown config keeps bare target", "ExampleServer", "Linux", "DebugGame", "", "ExampleServer"},
		{"config is case-insensitive and trimmed", "ExampleServer", "Linux", " shipping ", "", "ExampleServer-Linux-shipping"},
		{"Windows exe suffix on suffixed name", "ExampleServer", "Win64", "Shipping", ".exe", "ExampleServer-Win64-Shipping.exe"},
		{"Windows exe suffix on bare name", "ExampleServer", "Win64", "", ".exe", "ExampleServer.exe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanonicalServerBinaryName(tt.serverTarget, tt.binPlatform, tt.serverConfig, tt.executableSuffix)
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
		name             string
		binPlatform      string
		executableSuffix string
		staged           []string
		serverConfig     string
		wantName         string
	}{
		// #718: discovery must include the Shipping/Test suffix and search the
		// packaged directory (uproject basename), not the bare target.
		{name: "suffixed Shipping executable", binPlatform: "Linux", staged: []string{"ExampleServer-Linux-Shipping"}, wantName: "ExampleServer-Linux-Shipping"},
		{name: "bare Development executable", binPlatform: "Linux", staged: []string{"ExampleServer"}, wantName: "ExampleServer"},
		{name: "suffixed wins when both staged", binPlatform: "Linux", staged: []string{"ExampleServer", "ExampleServer-Linux-Shipping"}, wantName: "ExampleServer-Linux-Shipping"},
		{name: "multi-dot sidecars skipped", binPlatform: "Linux", staged: []string{"ExampleServer-Linux-Test.target", "ExampleServer-Linux-Test"}, wantName: "ExampleServer-Linux-Test"},
		{name: "arm64 binaries platform", binPlatform: "LinuxArm64", staged: []string{"ExampleServer-LinuxArm64-Shipping"}, serverConfig: "Shipping", wantName: "ExampleServer-LinuxArm64-Shipping"},
		{name: "canonical Shipping fallback when nothing staged", binPlatform: "Linux", staged: nil, serverConfig: "Shipping", wantName: "ExampleServer-Linux-Shipping"},
		{name: "bare fallback when nothing staged", binPlatform: "Linux", staged: nil, wantName: "ExampleServer"},
		// #744: a Windows host launches the wrapper directly, so executable
		// names carry ".exe" and a suffixed entry must actually carry it.
		{name: "Windows suffixed exe", binPlatform: "Win64", executableSuffix: ".exe", staged: []string{"ExampleServer-Win64-Shipping.exe"}, wantName: "ExampleServer-Win64-Shipping.exe"},
		{name: "Windows bare exe", binPlatform: "Win64", executableSuffix: ".exe", staged: []string{"ExampleServer.exe"}, wantName: "ExampleServer.exe"},
		{name: "Windows suffixed wins over bare", binPlatform: "Win64", executableSuffix: ".exe", staged: []string{"ExampleServer.exe", "ExampleServer-Win64-Shipping.exe"}, wantName: "ExampleServer-Win64-Shipping.exe"},
		{name: "Windows entry without exe suffix is skipped", binPlatform: "Win64", executableSuffix: ".exe", staged: []string{"ExampleServer-Win64-Shipping"}, wantName: "ExampleServer.exe"},
		{name: "Windows debug sidecar skipped", binPlatform: "Win64", executableSuffix: ".exe", staged: []string{"ExampleServer-Win64-Shipping.debug", "ExampleServer.exe"}, wantName: "ExampleServer.exe"},
		{name: "Windows canonical Shipping fallback", binPlatform: "Win64", executableSuffix: ".exe", staged: nil, serverConfig: "Shipping", wantName: "ExampleServer-Win64-Shipping.exe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverBuildDir := t.TempDir()
			stageServerBinaries(t, serverBuildDir, "ExampleContent", tt.binPlatform, tt.staged)
			got := DiscoverServerBinaryName(serverBuildDir, "ExampleContent", "ExampleServer", tt.binPlatform, tt.serverConfig, tt.executableSuffix)
			if got != tt.wantName {
				t.Errorf("DiscoverServerBinaryName() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestDiscoverServerBinary(t *testing.T) {
	serverBuildDir := t.TempDir()
	tests := []struct {
		name             string
		binPlatform      string
		executableSuffix string
		staged           []string
		wantFilename     string
	}{
		{name: "packaged dir differs from project name", binPlatform: "Linux", staged: nil, wantFilename: "ExampleServer"},
		{name: "discovers suffixed executable", binPlatform: "Linux", staged: []string{"ExampleServer-Linux-Shipping"}, wantFilename: "ExampleServer-Linux-Shipping"},
		{name: "arm64 path layout", binPlatform: "LinuxArm64", staged: []string{"ExampleServer-LinuxArm64-Shipping"}, wantFilename: "ExampleServer-LinuxArm64-Shipping"},
		{name: "Windows exe path layout", binPlatform: "Win64", executableSuffix: ".exe", staged: []string{"ExampleServer-Win64-Shipping.exe"}, wantFilename: "ExampleServer-Win64-Shipping.exe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := stageServerBinaries(t, serverBuildDir, "ExampleContent", tt.binPlatform, tt.staged)
			want := filepath.Join(binDir, tt.wantFilename)
			got := DiscoverServerBinary(serverBuildDir, "ExampleContent", "ExampleServer", tt.binPlatform, "", tt.executableSuffix)
			if got != want {
				t.Errorf("DiscoverServerBinary() = %q, want %q", got, want)
			}
		})
	}
}
