package globals

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestApplyEngineOverrides(t *testing.T) {
	base := &config.Config{
		Engine: config.EngineConfig{SourcePath: "/configured/engine", Backend: "native"},
	}

	tests := []struct {
		name        string
		backend     string
		sourcePath  string
		wantBackend string
		wantSource  string
	}{
		{
			name:        "non-empty overrides win",
			backend:     "wsl2",
			sourcePath:  "/override/engine",
			wantBackend: "wsl2",
			wantSource:  "/override/engine",
		},
		{
			name:        "empty overrides keep configured values",
			wantBackend: "native",
			wantSource:  "/configured/engine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base.Clone()
			ApplyEngineOverrides(&cfg, tt.backend, tt.sourcePath)
			if cfg.Engine.Backend != tt.wantBackend {
				t.Errorf("Backend = %q, want %q", cfg.Engine.Backend, tt.wantBackend)
			}
			if cfg.Engine.SourcePath != tt.wantSource {
				t.Errorf("SourcePath = %q, want %q", cfg.Engine.SourcePath, tt.wantSource)
			}
		})
	}
}

// assertOutputNil checks the nil-expectation for an output validator.
func assertOutputNil(t *testing.T, got cache.OutputExists, wantNil bool, path string) {
	t.Helper()
	if wantNil && got != nil {
		t.Errorf("output validator for %q = %v, want nil", path, got)
	}
}

func TestEngineBuildOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(engineHostBinariesDir(root), 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		wantNil bool
		want    bool
	}{
		{name: "existing host binaries dir", path: root, want: true},
		{name: "missing binaries dir", path: t.TempDir()},
		{name: "empty source path yields no validator", path: "", wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EngineBuildOutput(tt.path)
			assertOutputNil(t, got, tt.wantNil, tt.path)
			if tt.wantNil || got == nil {
				return
			}
			if got() != tt.want {
				t.Errorf("EngineBuildOutput(%q)() = %v, want %v", tt.path, got(), tt.want)
			}
		})
	}
}

func TestServerBuildOutput(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "MyGame", "MyGame.uproject")
	cfg := &config.Config{Game: config.GameConfig{ProjectName: "MyGame", ProjectPath: projectPath, Arch: "amd64"}}

	t.Run("missing packaged dir fails", func(t *testing.T) {
		if got := ServerBuildOutput(cfg); got == nil || got() {
			t.Error("ServerBuildOutput must fail while the packaged dir is absent")
		}
	})

	t.Run("existing packaged dir passes", func(t *testing.T) {
		if err := os.MkdirAll(config.ResolveServerBuildDir(cfg), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := ServerBuildOutput(cfg); got == nil || !got() {
			t.Error("ServerBuildOutput must pass once the packaged dir exists")
		}
	})

	t.Run("unresolvable project yields no validator", func(t *testing.T) {
		empty := &config.Config{Game: config.GameConfig{ProjectName: "Other"}}
		if got := ServerBuildOutput(empty); got != nil {
			t.Errorf("ServerBuildOutput() = %v, want nil for an unresolvable project", got)
		}
	})
}

func TestClientBuildOutput(t *testing.T) {
	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, "MyGame", "MyGame.uproject")
	cfg := &config.Config{Game: config.GameConfig{ProjectName: "MyGame", ProjectPath: projectPath}}

	tests := []struct {
		name     string
		platform string
		subdir   string
	}{
		{name: "linux", platform: "Linux", subdir: "Linux"},
		{name: "win64", platform: "Win64", subdir: "Windows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := filepath.Join(projectDir, "MyGame", "PackagedClient", tt.subdir)
			if err := os.MkdirAll(want, 0o755); err != nil {
				t.Fatal(err)
			}
			got := ClientBuildOutput(cfg, tt.platform)
			if got == nil || !got() {
				t.Errorf("ClientBuildOutput(%q) must pass for %q", tt.platform, want)
			}
		})
	}

	t.Run("missing output fails", func(t *testing.T) {
		cfg := &config.Config{Game: config.GameConfig{
			ProjectName: "MyGame",
			ProjectPath: filepath.Join(t.TempDir(), "P", "P.uproject"),
		}}
		if got := ClientBuildOutput(cfg, "Linux"); got == nil || got() {
			t.Error("ClientBuildOutput must fail while the packaged client dir is absent")
		}
	})

	t.Run("unresolvable project yields no validator", func(t *testing.T) {
		empty := &config.Config{Game: config.GameConfig{ProjectName: "Other"}}
		if got := ClientBuildOutput(empty, "Linux"); got != nil {
			t.Errorf("ClientBuildOutput() = %v, want nil for an unresolvable project", got)
		}
	})
}
