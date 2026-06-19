package config

import (
	"path/filepath"
	"testing"
)

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

func TestResolveServerArchiveRoot(t *testing.T) {
	tests := []struct {
		name        string
		projectPath string
		sourcePath  string
		projectName string
		arch        string
		want        string
	}{
		{
			name:        "custom project with projectPath (no platform dir)",
			projectPath: "/games/MyGame/MyGame.uproject",
			arch:        "amd64",
			want:        filepath.Join("/games/MyGame", "PackagedServer"),
		},
		{
			name:        "Lyra with engine source (no platform dir)",
			sourcePath:  "/engine",
			projectName: "Lyra",
			arch:        "arm64",
			want:        filepath.Join("/engine", "Samples", "Games", "Lyra", "PackagedServer"),
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
			got := ResolveServerArchiveRoot(cfg)
			if got != tt.want {
				t.Errorf("ResolveServerArchiveRoot() = %q, want %q", got, tt.want)
			}
			// Anti-doubling invariant: the build dir is exactly the archive root
			// plus one platform subdirectory. The Docker game builder appends the
			// platform itself, so it must be handed the root — never the build dir.
			if got != "" {
				want := filepath.Join(got, ServerPlatformDir(cfg.Game.ResolvedArch()))
				if bd := ResolveServerBuildDir(cfg); bd != want {
					t.Errorf("ResolveServerBuildDir() = %q, want archiveRoot+platform %q", bd, want)
				}
			}
		})
	}
}
