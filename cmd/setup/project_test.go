package setup

import (
	"path/filepath"
	"testing"
)

func TestPromptLyraContentEngineSample(t *testing.T) {
	t.Run("engine sample with discovered content accepted", func(t *testing.T) {
		enginePath := t.TempDir()
		createFile(t, filepath.Join(enginePath, "Samples", "Games", "Lyra", "Lyra.uproject"))
		home := t.TempDir()
		setTestHome(t, home)
		content := filepath.Join(home, "Documents", "Unreal Projects", "LyraStarterGame")
		createFile(t, filepath.Join(content, "Lyra.uproject"))
		withScannerInput(t, "\n")
		if got := promptLyraContent(enginePath); got != content {
			t.Errorf("got %q, want %q", got, content)
		}
	})
	t.Run("engine sample with declined content prompts", func(t *testing.T) {
		enginePath := t.TempDir()
		createFile(t, filepath.Join(enginePath, "Samples", "Games", "Lyra", "Lyra.uproject"))
		home := t.TempDir()
		setTestHome(t, home)
		createFile(t, filepath.Join(home, "Documents", "Unreal Projects", "LyraStarterGame", "Lyra.uproject"))
		withScannerInput(t, "n\n/manual/content\n")
		if got := promptLyraContent(enginePath); got != "/manual/content" {
			t.Errorf("got %q, want /manual/content", got)
		}
	})
	t.Run("engine sample with no content skips", func(t *testing.T) {
		enginePath := t.TempDir()
		createFile(t, filepath.Join(enginePath, "Samples", "Games", "Lyra", "Lyra.uproject"))
		setTestHome(t, t.TempDir())
		withScannerInput(t, "\n")
		if got := promptLyraContent(enginePath); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestPromptGameProjectDefaultLyraWithEngine(t *testing.T) {
	enginePath := t.TempDir()
	setTestHome(t, t.TempDir())
	withScannerInput(t, "Lyra\n/games/content\n")
	name, projectPath, content := promptGameProjectDefault(enginePath, "Lyra", nil)
	if name != "Lyra" || projectPath != "" || content != "/games/content" {
		t.Errorf("answers = %q, %q, %q", name, projectPath, content)
	}
}

func TestDiscoverLyraContentOneDrive(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	oneDrive := t.TempDir()
	t.Setenv("OneDrive", oneDrive)
	lyraDir := filepath.Join(oneDrive, "Documents", "Unreal Projects", "LyraStarterGame")
	createFile(t, filepath.Join(lyraDir, "Lyra.uproject"))
	if got := discoverLyraContent(); got != lyraDir {
		t.Errorf("got %q, want %q", got, lyraDir)
	}
}

func TestDiscoverLyraContentNoHome(t *testing.T) {
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	t.Setenv("HOME", "")
	if got := discoverLyraContent(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
