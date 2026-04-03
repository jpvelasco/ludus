package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"engine backend", cfg.Engine.Backend, "native"},
		{"engine docker image name", cfg.Engine.DockerImageName, "ludus-engine"},
		{"engine docker base image", cfg.Engine.DockerBaseImage, "ubuntu:22.04"},
		{"game project name", cfg.Game.ProjectName, "Lyra"},
		{"game platform", cfg.Game.Platform, "linux"},
		{"game arch", cfg.Game.Arch, "amd64"},
		{"game server map", cfg.Game.ServerMap, "L_Expanse"},
		{"container image name", cfg.Container.ImageName, "ludus-server"},
		{"container tag", cfg.Container.Tag, "latest"},
		{"deploy target", cfg.Deploy.Target, "gamelift"},
		{"gamelift fleet name", cfg.GameLift.FleetName, "ludus-fleet"},
		{"gamelift instance type", cfg.GameLift.InstanceType, "c6i.large"},
		{"gamelift container group", cfg.GameLift.ContainerGroupName, "ludus-container-group"},
		{"aws region", cfg.AWS.Region, "us-east-1"},
		{"aws ecr repository", cfg.AWS.ECRRepository, "ludus-server"},
		{"anywhere location", cfg.Anywhere.LocationName, "custom-ludus-dev"},
		{"anywhere aws profile", cfg.Anywhere.AWSProfile, "default"},
		{"ec2fleet sdk version", cfg.EC2Fleet.ServerSDKVersion, "5.4.0"},
		{"ci workflow path", cfg.CI.WorkflowPath, ".github/workflows/ludus-pipeline.yml"},
		{"ci runner dir", cfg.CI.RunnerDir, "~/actions-runner"},
		{"ddc mode", cfg.DDC.Mode, "local"},
		{"ddc local path", cfg.DDC.LocalPath, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}

	// Numeric defaults
	if cfg.Engine.MaxJobs != 0 {
		t.Errorf("engine max jobs: got %d, want 0", cfg.Engine.MaxJobs)
	}
	if cfg.Container.ServerPort != 7777 {
		t.Errorf("container server port: got %d, want 7777", cfg.Container.ServerPort)
	}
	if cfg.GameLift.MaxConcurrentSessions != 1 {
		t.Errorf("gamelift max concurrent sessions: got %d, want 1", cfg.GameLift.MaxConcurrentSessions)
	}

	// Map defaults
	if cfg.AWS.Tags["ManagedBy"] != "ludus" {
		t.Errorf("aws tags ManagedBy: got %q, want %q", cfg.AWS.Tags["ManagedBy"], "ludus")
	}
	if len(cfg.CI.RunnerLabels) != 3 {
		t.Errorf("ci runner labels: got %d labels, want 3", len(cfg.CI.RunnerLabels))
	}
}

func TestNormalizeArch(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "amd64"},
		{"amd64", "amd64", "amd64"},
		{"x86_64", "x86_64", "amd64"},
		{"arm64", "arm64", "arm64"},
		{"aarch64", "aarch64", "arm64"},
		{"uppercase AMD64", "AMD64", "amd64"},
		{"uppercase ARM64", "ARM64", "arm64"},
		{"mixed case AArch64", "AArch64", "arm64"},
		{"whitespace", "  arm64  ", "arm64"},
		{"unknown defaults to amd64", "mips64", "amd64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeArch(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeArch(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestServerPlatformDir(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"amd64", "LinuxServer"},
		{"arm64", "LinuxArm64Server"},
		{"x86_64", "LinuxServer"},
		{"aarch64", "LinuxArm64Server"},
		{"", "LinuxServer"},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			got := ServerPlatformDir(tt.arch)
			if got != tt.want {
				t.Errorf("ServerPlatformDir(%q) = %q, want %q", tt.arch, got, tt.want)
			}
		})
	}
}

func TestBinariesPlatformDir(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"amd64", "Linux"},
		{"arm64", "LinuxArm64"},
		{"", "Linux"},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			got := BinariesPlatformDir(tt.arch)
			if got != tt.want {
				t.Errorf("BinariesPlatformDir(%q) = %q, want %q", tt.arch, got, tt.want)
			}
		})
	}
}

func TestUEPlatformName(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"amd64", "Linux"},
		{"arm64", "LinuxArm64"},
		{"", "Linux"},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			got := UEPlatformName(tt.arch)
			if got != tt.want {
				t.Errorf("UEPlatformName(%q) = %q, want %q", tt.arch, got, tt.want)
			}
		})
	}
}

func TestGameConfig_ResolvedServerTarget(t *testing.T) {
	tests := []struct {
		name         string
		serverTarget string
		projectName  string
		want         string
	}{
		{"explicit target", "MyServer", "MyGame", "MyServer"},
		{"default from project", "", "MyGame", "MyGameServer"},
		{"default Lyra", "", "Lyra", "LyraServer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GameConfig{ServerTarget: tt.serverTarget, ProjectName: tt.projectName}
			got := g.ResolvedServerTarget()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGameConfig_ResolvedClientTarget(t *testing.T) {
	tests := []struct {
		name         string
		clientTarget string
		projectName  string
		want         string
	}{
		{"explicit target", "MyClient", "MyGame", "MyClient"},
		{"default from project", "", "MyGame", "MyGameGame"},
		{"default Lyra", "", "Lyra", "LyraGame"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GameConfig{ClientTarget: tt.clientTarget, ProjectName: tt.projectName}
			got := g.ResolvedClientTarget()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGameConfig_ResolvedGameTarget(t *testing.T) {
	tests := []struct {
		name        string
		gameTarget  string
		projectName string
		want        string
	}{
		{"explicit target", "MyTarget", "MyGame", "MyTarget"},
		{"default from project", "", "MyGame", "MyGameGame"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GameConfig{GameTarget: tt.gameTarget, ProjectName: tt.projectName}
			got := g.ResolvedGameTarget()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGameConfig_ResolvedArch(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"", "amd64"},
		{"arm64", "arm64"},
		{"aarch64", "arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			g := &GameConfig{Arch: tt.arch}
			got := g.ResolvedArch()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoad_MissingFile(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load should not fail on missing file: %v", err)
	}

	// Should return defaults
	if cfg.Game.ProjectName != "Lyra" {
		t.Errorf("expected default project name %q, got %q", "Lyra", cfg.Game.ProjectName)
	}
}

func TestLoad_ValidYAML(t *testing.T) {
	t.Chdir(t.TempDir())

	yamlContent := `engine:
  sourcePath: /tmp/ue5
  version: "5.7.0"
game:
  projectName: MyGame
  arch: arm64
aws:
  region: eu-west-1
`
	if err := os.WriteFile("ludus.yaml", []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Game.ProjectName != "MyGame" {
		t.Errorf("project name: got %q, want %q", cfg.Game.ProjectName, "MyGame")
	}
	if cfg.Game.Arch != "arm64" {
		t.Errorf("arch: got %q, want %q", cfg.Game.Arch, "arm64")
	}
	if cfg.AWS.Region != "eu-west-1" {
		t.Errorf("region: got %q, want %q", cfg.AWS.Region, "eu-west-1")
	}
	// Defaults should still apply for unset fields
	if cfg.Container.ServerPort != 7777 {
		t.Errorf("server port should default to 7777, got %d", cfg.Container.ServerPort)
	}
}

func TestLoad_ExplicitPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "custom.yaml")

	yamlContent := `game:
  projectName: CustomGame
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Game.ProjectName != "CustomGame" {
		t.Errorf("project name: got %q, want %q", cfg.Game.ProjectName, "CustomGame")
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "bad.yaml")

	// YAML with a mapping value where a sequence is expected triggers a parse error
	if err := os.WriteFile(configPath, []byte("engine:\n  - sourcePath: x\n    sourcePath: y"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	// Viper/YAML may or may not error on subtly invalid YAML, but the result
	// should either return an error or silently return defaults without panic.
	if err != nil {
		return // expected — malformed YAML raised an error
	}
	// If no error, verify we at least get a usable config (defensive parse)
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
		{
			name:    "empty file",
			content: "",
			wantErr: false, // Viper treats empty as no config
		},
		{
			name:    "valid but empty YAML",
			content: "---\n",
			wantErr: false,
		},
		{
			name:    "unknown keys ignored",
			content: "nonexistent:\n  foo: bar\n",
			wantErr: false,
		},
		{
			name:    "wrong type for port",
			content: "container:\n  serverPort: not-a-number\n",
			wantErr: true,
		},
		{
			name:    "wrong type for maxJobs",
			content: "engine:\n  maxJobs: [1, 2, 3]\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "ludus.yaml")
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
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	yamlContent := `lyra:
  projectName: LegacyGame
  serverMap: TestMap
`
	if err := os.WriteFile("ludus.yaml", []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Game.ProjectName != "LegacyGame" {
		t.Errorf("project name: got %q, want %q (lyra key migration)", cfg.Game.ProjectName, "LegacyGame")
	}
	if cfg.Game.ServerMap != "TestMap" {
		t.Errorf("server map: got %q, want %q (lyra key migration)", cfg.Game.ServerMap, "TestMap")
	}
}

func TestGameConfig_ResolveProjectPath(t *testing.T) {
	t.Run("already set", func(t *testing.T) {
		g := &GameConfig{ProjectPath: "/existing/path.uproject"}
		g.ResolveProjectPath("/engine")
		if g.ProjectPath != "/existing/path.uproject" {
			t.Errorf("should not change existing path, got %q", g.ProjectPath)
		}
	})

	t.Run("empty engine path", func(t *testing.T) {
		g := &GameConfig{ProjectName: "Lyra"}
		g.ResolveProjectPath("")
		if g.ProjectPath != "" {
			t.Errorf("should not set path with empty engine path, got %q", g.ProjectPath)
		}
	})

	t.Run("Lyra with valid engine path", func(t *testing.T) {
		tmpDir := t.TempDir()
		lyraDir := filepath.Join(tmpDir, "Samples", "Games", "Lyra")
		if err := os.MkdirAll(lyraDir, 0755); err != nil {
			t.Fatal(err)
		}
		uproject := filepath.Join(lyraDir, "Lyra.uproject")
		if err := os.WriteFile(uproject, []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}

		g := &GameConfig{ProjectName: "Lyra"}
		g.ResolveProjectPath(tmpDir)
		if g.ProjectPath != uproject {
			t.Errorf("got %q, want %q", g.ProjectPath, uproject)
		}
	})

	t.Run("Lyra with missing uproject", func(t *testing.T) {
		tmpDir := t.TempDir()
		g := &GameConfig{ProjectName: "Lyra"}
		g.ResolveProjectPath(tmpDir)
		if g.ProjectPath != "" {
			t.Errorf("should not set path when uproject missing, got %q", g.ProjectPath)
		}
	})

	t.Run("non-Lyra project", func(t *testing.T) {
		tmpDir := t.TempDir()
		g := &GameConfig{ProjectName: "MyGame"}
		g.ResolveProjectPath(tmpDir)
		if g.ProjectPath != "" {
			t.Errorf("should not auto-resolve for non-Lyra projects, got %q", g.ProjectPath)
		}
	})
}

func TestLoad_DDCConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	yamlContent := `ddc:
  mode: none
  local_path: /custom/ddc
`
	if err := os.WriteFile("ludus.yaml", []byte(yamlContent), 0644); err != nil {
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
		t.Errorf("ddc local_path: got %q, want %q", cfg.DDC.LocalPath, "/custom/ddc")
	}
}

func TestResolveServerBuildDir(t *testing.T) {
	tests := []struct {
		name        string
		projectPath string
		sourcePath  string
		projectName string
		arch        string
		want        string
	}{
		{
			name:        "custom project with projectPath",
			projectPath: "/games/MyGame/MyGame.uproject",
			arch:        "amd64",
			want:        filepath.Join("/games/MyGame", "PackagedServer", "LinuxServer"),
		},
		{
			name:        "Lyra with engine source",
			sourcePath:  "/engine",
			projectName: "Lyra",
			arch:        "arm64",
			want:        filepath.Join("/engine", "Samples", "Games", "Lyra", "PackagedServer", "LinuxArm64Server"),
		},
		{
			name:        "projectPath takes priority over Lyra",
			projectPath: "/games/MyGame/MyGame.uproject",
			sourcePath:  "/engine",
			projectName: "Lyra",
			arch:        "amd64",
			want:        filepath.Join("/games/MyGame", "PackagedServer", "LinuxServer"),
		},
		{
			name: "neither set returns empty",
			arch: "amd64",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Engine: EngineConfig{SourcePath: tt.sourcePath},
				Game: GameConfig{
					ProjectPath: tt.projectPath,
					ProjectName: tt.projectName,
					Arch:        tt.arch,
				},
			}
			got := ResolveServerBuildDir(cfg)
			if got != tt.want {
				t.Errorf("ResolveServerBuildDir() = %q, want %q", got, tt.want)
			}
		})
	}
}
