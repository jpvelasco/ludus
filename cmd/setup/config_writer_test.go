package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConfigError(t *testing.T) {
	answers := setupAnswers{
		cfgFile:      filepath.Join(t.TempDir(), "missing", "ludus.yaml"),
		projectName:  "Game",
		deployTarget: "gamelift",
	}
	if err := writeConfig(answers); err == nil {
		t.Error("writeConfig() expected error for missing directory")
	}
}

func TestWriteConfigRegionDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	answers := setupAnswers{
		cfgFile:      "ludus.yaml",
		projectName:  "Game",
		deployTarget: "gamelift",
		instanceType: "c6i.large",
	}
	if err := writeConfig(answers); err != nil {
		t.Fatalf("writeConfig() error: %v", err)
	}
	data, err := os.ReadFile("ludus.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "region: us-east-1") {
		t.Errorf("expected default region us-east-1:\n%s", data)
	}
}

func TestWriteConfigProjectPath(t *testing.T) {
	t.Chdir(t.TempDir())
	answers := setupAnswers{
		cfgFile:      "ludus.yaml",
		projectName:  "Game",
		projectPath:  "/games/Game.uproject",
		deployTarget: "binary",
		region:       "us-west-2",
	}
	if err := writeConfig(answers); err != nil {
		t.Fatalf("writeConfig() error: %v", err)
	}
	data, err := os.ReadFile("ludus.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "projectpath: /games/Game.uproject") {
		t.Errorf("expected projectPath in config:\n%s", data)
	}
}

func TestPreserveExistingTags(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("ludus.yaml", []byte("aws:\n  tags:\n    Team: games\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	answers := setupAnswers{
		cfgFile:      "ludus.yaml",
		projectName:  "Game",
		deployTarget: "gamelift",
		region:       "us-east-1",
		instanceType: "c6i.large",
	}
	if err := writeConfig(answers); err != nil {
		t.Fatalf("writeConfig() error: %v", err)
	}
	data, err := os.ReadFile("ludus.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// Viper lowercases map keys on the round-trip; the value must survive.
	if !strings.Contains(string(data), "games") {
		t.Errorf("expected preserved aws.tags:\n%s", data)
	}
}
