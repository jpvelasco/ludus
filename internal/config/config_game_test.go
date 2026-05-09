package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
		g := &GameConfig{ProjectName: "Lyra"}
		g.ResolveProjectPath(t.TempDir())
		if g.ProjectPath != "" {
			t.Errorf("should not set path when uproject missing, got %q", g.ProjectPath)
		}
	})

	t.Run("non-Lyra project", func(t *testing.T) {
		g := &GameConfig{ProjectName: "MyGame"}
		g.ResolveProjectPath(t.TempDir())
		if g.ProjectPath != "" {
			t.Errorf("should not auto-resolve for non-Lyra projects, got %q", g.ProjectPath)
		}
	})
}
