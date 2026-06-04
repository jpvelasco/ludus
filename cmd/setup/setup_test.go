package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
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

func TestWriteConfig_PreservesUnpromptedFields(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	existing := &config.Config{}
	existing.Engine.Backend = "podman"
	existing.Engine.MaxJobs = 8
	existing.DDC.Mode = "local"
	existing.DDC.LocalPath = "/custom/ddc"
	existing.Game.Arch = "arm64"

	answers := setupAnswers{
		cfgFile:      "ludus.yaml",
		enginePath:   "/opt/UnrealEngine",
		projectName:  "Lyra",
		deployTarget: "gamelift",
		region:       "eu-west-1",
		instanceType: "c6i.large",
	}

	if err := writeConfig(answers, existing); err != nil {
		t.Fatalf("writeConfig() error: %v", err)
	}

	data, err := os.ReadFile("ludus.yaml")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)

	// Wizard fields written correctly
	if !strings.Contains(content, "region: eu-west-1") {
		t.Errorf("expected wizard-set region in config:\n%s", content)
	}
	// Un-prompted fields preserved from existing config
	for _, want := range []string{
		"backend: podman",
		"maxjobs: 8",
		"mode: local",
		"localpath: /custom/ddc",
		"arch: arm64",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected preserved field %q in config:\n%s", want, content)
		}
	}
}

func TestWriteConfig_NilExistingIsFirstRun(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	answers := setupAnswers{
		cfgFile:      "ludus.yaml",
		projectName:  "Lyra",
		deployTarget: "gamelift",
		region:       "us-east-1",
		instanceType: "c6i.large",
	}
	// nil existing should not panic
	if err := writeConfig(answers, nil); err != nil {
		t.Fatalf("writeConfig(nil existing) error: %v", err)
	}
}

func TestWriteConfig(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	answers := setupAnswers{
		cfgFile:           "ludus.yaml",
		enginePath:        "/opt/UnrealEngine",
		engineVersion:     "5.7.3",
		projectName:       "Lyra",
		contentSourcePath: "/games/LyraStarterGame",
		deployTarget:      "gamelift",
		region:            "us-west-2",
		accountID:         "123456789012",
		instanceType:      "c6i.xlarge",
	}

	if err := writeConfig(answers, nil); err != nil {
		t.Fatalf("writeConfig() error: %v", err)
	}

	data, err := os.ReadFile("ludus.yaml")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"sourcepath: /opt/UnrealEngine",
		"version: 5.7.3",
		"projectname: Lyra",
		"contentsourcepath: /games/LyraStarterGame",
		"target: gamelift",
		"region: us-west-2",
		"accountid: \"123456789012\"",
		"instancetype: c6i.xlarge",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config missing %q:\n%s", want, content)
		}
	}
}
