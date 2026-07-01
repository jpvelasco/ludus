package setup

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestPromptHelpers(t *testing.T) {
	tests := []struct {
		name, input, want string
		run               func() string
	}{
		{"prompt default", "\n", "fallback", func() string { return prompt("Question", "fallback") }},
		{"prompt answer", " value \n", "value", func() string { return prompt("Question", "") }},
		{"choice default", "\n", "one", func() string { return promptChoice("Pick", []string{"one", "two"}, 0) }},
		{"choice number", "2\n", "two", func() string { return promptChoice("Pick", []string{"one", "two"}, 0) }},
		{"choice name", "TWO\n", "two", func() string { return promptChoice("Pick", []string{"one", "two"}, 0) }},
		{"choice invalid", "9\n", "one", func() string { return promptChoice("Pick", []string{"one", "two"}, 0) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withScannerInput(t, tt.input)
			if got := tt.run(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfirm(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  bool
	}{{"\n", true}, {"yes\n", true}, {"Y\n", true}, {"no\n", false}} {
		withScannerInput(t, tt.input)
		if got := confirm("Continue?"); got != tt.want {
			t.Errorf("confirm(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestProjectPrompts(t *testing.T) {
	t.Run("custom project", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "Game.uproject")
		createFile(t, path)
		withScannerInput(t, "Game\n"+path+"\n")
		name, projectPath, content := promptGameProjectDefault("", "Lyra", nil)
		if name != "Game" || projectPath != path || content != "" {
			t.Errorf("answers = %q, %q, %q", name, projectPath, content)
		}
	})
	t.Run("missing custom project", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing.uproject")
		withScannerInput(t, missing+"\n")
		output := captureSetupStdout(t, func() { promptCustomProjectDefault("") })
		if !strings.Contains(output, "Warning:") {
			t.Errorf("output = %q, want warning", output)
		}
	})
	t.Run("existing Lyra content accepted", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)
		content := filepath.Join(home, "Documents", "Unreal Projects", "LyraStarterGame")
		createFile(t, filepath.Join(content, "Lyra.uproject"))
		withScannerInput(t, "\n")
		if got := promptLyraContent(t.TempDir()); got != content {
			t.Errorf("got %q, want %q", got, content)
		}
	})
}

func TestResolveConfigFileAndSummary(t *testing.T) {
	previous := globals.Profile
	globals.Profile = "demo"
	t.Cleanup(func() { globals.Profile = previous })
	if got := resolveConfigFile(); got != "ludus-demo.yaml" {
		t.Errorf("resolveConfigFile() = %q", got)
	}
	globals.Profile = ""
	if got := resolveConfigFile(); got != "ludus.yaml" {
		t.Errorf("resolveConfigFile() = %q", got)
	}

	output := captureSetupStdout(t, func() {
		printSummary(setupAnswers{
			cfgFile: "ludus.yaml", projectName: "Game", projectPath: "Game.uproject",
			deployTarget: "binary", region: "us-west-2",
		})
	})
	for _, want := range []string{"Configuration Summary", "Game.uproject", "binary", "us-west-2", "ludus.yaml"} {
		if !strings.Contains(output, want) {
			t.Errorf("summary missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Instance type:") {
		t.Errorf("binary summary unexpectedly contains instance type:\n%s", output)
	}
}

func TestCollectDeploymentAndAWSAnswers(t *testing.T) {
	existing := &config.Config{}
	existing.Deploy.Target = "stack"
	existing.AWS.Region = "eu-west-1"
	existing.GameLift.InstanceType = "c7i.large"
	withScannerInput(t, "\n\n\n")
	var answers setupAnswers
	collectDeploymentAnswers(&answers, existing)
	collectAWSAnswers(&answers, existing)
	if answers.deployTarget != "stack" || answers.region != "eu-west-1" || answers.instanceType != "c7i.large" {
		t.Errorf("answers = %+v", answers)
	}
}

func captureSetupStdout(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	run()
	os.Stdout = previous
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
