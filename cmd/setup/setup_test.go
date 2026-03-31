package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// createFile creates a zero-byte file, creating parent directories as needed.
func createFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// setTestHome overrides the home directory environment variable for the test.
func setTestHome(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	} else {
		t.Setenv("HOME", dir)
	}
	// Clear OneDrive so discovery doesn't pick up real user paths
	t.Setenv("OneDrive", "")
}

var isLyraProjectTests = []struct {
	name  string
	files []string // relative paths to create inside project dir
	want  bool
}{
	{name: "uproject marker", files: []string{"Lyra.uproject"}, want: true},
	{name: "content marker", files: []string{filepath.Join("Content", "DefaultGameData.uasset")}, want: true},
	{name: "both markers", files: []string{"Lyra.uproject", filepath.Join("Content", "DefaultGameData.uasset")}, want: true},
	{name: "empty directory", want: false},
	{name: "wrong uproject", files: []string{"Other.uproject"}, want: false},
}

func TestIsLyraProject(t *testing.T) {
	for _, tt := range isLyraProjectTests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				createFile(t, filepath.Join(dir, f))
			}
			if got := isLyraProject(dir); got != tt.want {
				t.Errorf("isLyraProject() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiscoverLyraContent(t *testing.T) {
	t.Run("no content", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		if got := discoverLyraContent(); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("standard path", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)
		lyraDir := filepath.Join(home, "Documents", "Unreal Projects", "LyraStarterGame")
		createFile(t, filepath.Join(lyraDir, "Lyra.uproject"))
		if got := discoverLyraContent(); got != lyraDir {
			t.Errorf("got %q, want %q", got, lyraDir)
		}
	})

	t.Run("versioned fallback", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)
		lyraDir := filepath.Join(home, "Documents", "Unreal Projects", "LyraStarterGame_5.7")
		createFile(t, filepath.Join(lyraDir, "Lyra.uproject"))
		got := discoverLyraContent()
		if !strings.HasSuffix(got, "LyraStarterGame_5.7") {
			t.Errorf("got %q, want suffix LyraStarterGame_5.7", got)
		}
	})
}
