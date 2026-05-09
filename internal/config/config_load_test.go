package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFile(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load should not fail on missing file: %v", err)
	}
	if cfg.Game.ProjectName != "Lyra" {
		t.Errorf("expected default project name %q, got %q", "Lyra", cfg.Game.ProjectName)
	}
}

func TestLoad_ValidYAML(t *testing.T) {
	t.Chdir(t.TempDir())

	yaml := `engine:
  sourcePath: /tmp/ue5
  version: "5.7.0"
game:
  projectName: MyGame
  arch: arm64
aws:
  region: eu-west-1
`
	if err := os.WriteFile("ludus.yaml", []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"projectName", cfg.Game.ProjectName, "MyGame"},
		{"arch", cfg.Game.Arch, "arm64"},
		{"region", cfg.AWS.Region, "eu-west-1"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, c.got, c.want)
		}
	}
	if cfg.Container.ServerPort != 7777 {
		t.Errorf("serverPort should default to 7777, got %d", cfg.Container.ServerPort)
	}
}

func TestLoad_ExplicitPath(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(configPath, []byte("game:\n  projectName: CustomGame\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Game.ProjectName != "CustomGame" {
		t.Errorf("projectName: got %q, want %q", cfg.Game.ProjectName, "CustomGame")
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(configPath, []byte("engine:\n  - sourcePath: x\n    sourcePath: y"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		return // expected — malformed YAML raised an error
	}
	if cfg == nil {
		t.Fatal("expected non-nil config even for lenient parse")
	}
}

func TestLoad_NegativeCases(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{"empty file", "", false},
		{"valid but empty YAML", "---\n", false},
		{"unknown keys ignored", "nonexistent:\n  foo: bar\n", false},
		{"wrong type for port", "container:\n  serverPort: not-a-number\n", true},
		{"wrong type for maxJobs", "engine:\n  maxJobs: [1, 2, 3]\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "ludus.yaml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(configPath)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoad_DeprecatedLyraKey(t *testing.T) {
	t.Chdir(t.TempDir())

	yaml := `lyra:
  projectName: LegacyGame
  serverMap: TestMap
`
	if err := os.WriteFile("ludus.yaml", []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Game.ProjectName != "LegacyGame" {
		t.Errorf("projectName: got %q, want %q (lyra key migration)", cfg.Game.ProjectName, "LegacyGame")
	}
	if cfg.Game.ServerMap != "TestMap" {
		t.Errorf("serverMap: got %q, want %q (lyra key migration)", cfg.Game.ServerMap, "TestMap")
	}
}

func TestLoad_DDCConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	yaml := `ddc:
  mode: none
  localPath: /custom/ddc
`
	if err := os.WriteFile("ludus.yaml", []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DDC.Mode != "none" {
		t.Errorf("ddc mode: got %q, want %q", cfg.DDC.Mode, "none")
	}
	if cfg.DDC.LocalPath != "/custom/ddc" {
		t.Errorf("ddc localPath: got %q, want %q", cfg.DDC.LocalPath, "/custom/ddc")
	}
}
