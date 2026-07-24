package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestScanWorkingTreeEnginePaths(t *testing.T) {
	root := t.TempDir()
	working := filepath.Join(root, "project")
	createDirectory(t, working)
	valid := createEngineCandidate(t, root, "UnrealEngine-5.8")
	createDirectory(t, filepath.Join(root, "UnrealEngine-invalid"))
	t.Chdir(working)

	var got []string
	scanWorkingTreeEnginePaths(func(path string) {
		if isEngineSourceDir(path) {
			got = append(got, path)
		}
	})
	if !reflect.DeepEqual(got, []string{valid}) {
		t.Fatalf("candidates = %v, want [%s]", got, valid)
	}
}

func TestScanHomeEnginePathsOrdering(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	docCandidate := createEngineCandidate(t, filepath.Join(home, "Documents", "Source"), "UnrealEngine-docs")
	sourceCandidate := createEngineCandidate(t, filepath.Join(home, "Source"), "UnrealEngine-source")

	var got []string
	scanHomeEnginePaths(func(path string) {
		if isEngineSourceDir(path) {
			got = append(got, path)
		}
	})
	want := []string{docCandidate, sourceCandidate}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}

func TestScanGlobMalformedPattern(t *testing.T) {
	called := false
	scanGlob(t.TempDir(), "[", func(string) { called = true })
	if called {
		t.Error("callback called for malformed pattern")
	}
}

func TestReadEngineCandidateChoice(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "default", input: "\n", want: "1"},
		{name: "trimmed choice", input: " 3 \n", want: "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withScannerInput(t, tt.input)
			if got := readEngineCandidateChoice(); got != tt.want {
				t.Errorf("choice = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPromptEnginePathDefaultChoice(t *testing.T) {
	root := t.TempDir()
	working := filepath.Join(root, "project")
	createDirectory(t, working)
	want := createEngineCandidate(t, root, "UnrealEngine-choice")
	t.Chdir(working)
	t.Setenv("USERPROFILE", filepath.Join(root, "empty-home"))
	t.Setenv("HOME", filepath.Join(root, "empty-home"))
	withScannerInput(t, "1\n")
	if got := promptEnginePathDefault(""); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func createEngineCandidate(t *testing.T, parent, name string) string {
	t.Helper()
	path := filepath.Join(parent, name)
	createDirectory(t, path)
	if err := os.WriteFile(filepath.Join(path, engineSetupFile()), []byte("setup"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func createDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
