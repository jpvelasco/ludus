package setup

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPromptEnginePathDefaultBranches(t *testing.T) {
	t.Run("no candidates prompts", func(t *testing.T) {
		t.Chdir(t.TempDir())
		setTestHome(t, t.TempDir())
		withScannerInput(t, "\n")
		if got := promptEnginePathDefault("fallback"); got != "fallback" {
			t.Errorf("got %q, want fallback", got)
		}
	})
	t.Run("enter different path", func(t *testing.T) {
		_, _ = engineChoiceRoot(t)
		withScannerInput(t, "2\n/manual/path\n")
		if got := promptEnginePathDefault("defpath"); got != "/manual/path" {
			t.Errorf("got %q, want /manual/path", got)
		}
	})
	t.Run("invalid choice uses default", func(t *testing.T) {
		_, _ = engineChoiceRoot(t)
		withScannerInput(t, "9\n")
		if got := promptEnginePathDefault("defpath"); got != "defpath" {
			t.Errorf("got %q, want defpath", got)
		}
	})
}

func TestScanEnginePathsFiltersNonEngine(t *testing.T) {
	root := t.TempDir()
	working := filepath.Join(root, "project")
	createDirectory(t, working)
	createDirectory(t, filepath.Join(root, "UnrealEngine-no-setup"))
	valid := createEngineCandidate(t, root, "UnrealEngine-real")
	t.Chdir(working)
	setTestHome(t, filepath.Join(root, "empty-home"))

	got := scanEnginePaths()
	if !reflect.DeepEqual(got, []string{valid}) {
		t.Fatalf("scanEnginePaths() = %v, want [%s]", got, valid)
	}
}

// TestScanHomeEnginePathsEmptyHome forces os.UserHomeDir to fail by blanking
// every Windows home env var, exercising the home == "" early return.
func TestScanHomeEnginePathsEmptyHome(t *testing.T) {
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	t.Setenv("HOME", "")
	called := false
	scanHomeEnginePaths(func(string) { called = true })
	if called {
		t.Error("callback called with empty home")
	}
}
