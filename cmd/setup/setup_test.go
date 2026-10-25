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

// TestWriteConfig_PreservesUnpromptedFields reproduces #703: a wizard re-run
// must seed from the existing config document so every setting it never
// prompts for survives, and must not overwrite a custom ECR repository with
// the default.
func TestWriteConfig_PreservesUnpromptedFields(t *testing.T) {
	t.Chdir(t.TempDir())
	const seed = `engine:
  backend: podman
  maxJobs: 8
game:
  arch: arm64
  serverMap: MyCustomMap
ddc:
  mode: zen
  zenPath: /custom/zen
  localPath: /custom/ddc
privacy:
  maskAccountId: false
observability:
  logs:
    dir: /custom/logs
    retainRuns: 5
deploy:
  target: stack
  outputDir: /custom/out
ec2fleet:
  s3Bucket: my-bucket
  allowedCidr: 10.0.0.0/8
aws:
  accountId: "111122223333"
  ecrRepository: custom-repo
gamelift:
  fleetName: custom-fleet
container:
  imageName: custom-image
`
	if err := os.WriteFile("ludus.yaml", []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	answers := setupAnswers{
		cfgFile:      "ludus.yaml",
		enginePath:   "/opt/UnrealEngine",
		projectName:  "Lyra",
		deployTarget: "gamelift",
		region:       "eu-west-1",
		instanceType: "c6i.large",
	}
	if err := writeConfig(answers); err != nil {
		t.Fatalf("writeConfig() error: %v", err)
	}

	data, err := os.ReadFile("ludus.yaml")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)

	assertConfigContains(t, content, []string{
		"sourcepath: /opt/UnrealEngine",
		"projectname: Lyra",
		"target: gamelift",
		"region: eu-west-1",
	}, "wizard value")
	assertConfigContains(t, content, unpromptedConfigFields, "preserved field")
	if strings.Contains(content, "ecrrepository: ludus-server") {
		t.Errorf("custom ECR repository was overwritten with the default:\n%s", content)
	}
}

// unpromptedConfigFields are settings the wizard never asks about; a re-run
// must carry them over from the existing document (#703).
var unpromptedConfigFields = []string{
	"backend: podman",
	"maxjobs: 8",
	"arch: arm64",
	"servermap: MyCustomMap",
	"mode: zen",
	"zenpath: /custom/zen",
	"localpath: /custom/ddc",
	"maskaccountid: false",
	"dir: /custom/logs",
	"retainruns: 5",
	"outputdir: /custom/out",
	"s3bucket: my-bucket",
	"allowedcidr: 10.0.0.0/8",
	"accountid: \"111122223333\"",
	"ecrrepository: custom-repo",
	"fleetname: custom-fleet",
	"imagename: custom-image",
}

func assertConfigContains(t *testing.T, content string, wants []string, label string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(content, want) {
			t.Errorf("config missing %s %q:\n%s", label, want, content)
		}
	}
}

// TestWriteConfigUnreadableFileStartsFresh pins the runSetup contract: the
// wizard only reaches writeConfig with an unreadable file after the user
// confirms the overwrite, so the write starts from a blank document.
func TestWriteConfigUnreadableFileStartsFresh(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("ludus.yaml", []byte("engine:\n  version: [5.8\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	answers := setupAnswers{
		cfgFile:      "ludus.yaml",
		projectName:  "Game",
		deployTarget: "gamelift",
		instanceType: "c6i.large",
	}
	if err := writeConfig(answers); err != nil {
		t.Fatalf("writeConfig() error = %v, want the confirmed overwrite to proceed", err)
	}

	data, err := os.ReadFile("ludus.yaml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "projectname: Game") {
		t.Errorf("config missing wizard values after fresh write:\n%s", content)
	}
	if strings.Contains(content, "version: [5.8") {
		t.Errorf("unreadable content should have been replaced:\n%s", content)
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
	// no existing file: the wizard writes a fresh document
	if err := writeConfig(answers); err != nil {
		t.Fatalf("writeConfig() error: %v", err)
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

	if err := writeConfig(answers); err != nil {
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
