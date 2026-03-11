package container

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProjectName(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		want        string
	}{
		{"explicit", "MyGame", "MyGame"},
		{"empty defaults to Lyra", "", "Lyra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(BuildOptions{ProjectName: tt.projectName}, nil)
			got := b.resolveProjectName()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveServerTarget(t *testing.T) {
	tests := []struct {
		name         string
		serverTarget string
		projectName  string
		want         string
	}{
		{"explicit", "CustomServer", "", "CustomServer"},
		{"default from project", "", "MyGame", "MyGameServer"},
		{"default Lyra", "", "", "LyraServer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(BuildOptions{
				ServerTarget: tt.serverTarget,
				ProjectName:  tt.projectName,
			}, nil)
			got := b.resolveServerTarget()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveArch(t *testing.T) {
	tests := []struct {
		name string
		arch string
		want string
	}{
		{"empty defaults to amd64", "", "amd64"},
		{"amd64", "amd64", "amd64"},
		{"arm64", "arm64", "arm64"},
		{"aarch64 normalized", "aarch64", "arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(BuildOptions{Arch: tt.arch}, nil)
			got := b.resolveArch()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateDockerfile(t *testing.T) {
	tests := []struct {
		name        string
		opts        BuildOptions
		wantContain []string
	}{
		{
			name: "amd64 defaults",
			opts: BuildOptions{
				ServerPort:  7777,
				ProjectName: "Lyra",
			},
			wantContain: []string{
				"FROM public.ecr.aws/amazonlinux/amazonlinux:2023",
				"Lyra/Binaries/Linux/LyraServer",
				"EXPOSE 7777/udp",
				"USER ueserver",
				"ENTRYPOINT [\"./amazon-gamelift-servers-game-server-wrapper\"]",
			},
		},
		{
			name: "arm64",
			opts: BuildOptions{
				ServerPort:   7777,
				ProjectName:  "MyGame",
				ServerTarget: "MyGameServer",
				Arch:         "arm64",
			},
			wantContain: []string{
				"MyGame/Binaries/LinuxArm64/MyGameServer",
				"EXPOSE 7777/udp",
			},
		},
		{
			name: "custom port",
			opts: BuildOptions{
				ServerPort:  9999,
				ProjectName: "Test",
			},
			wantContain: []string{
				"EXPOSE 9999/udp",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(tt.opts, nil)
			dockerfile := b.GenerateDockerfile()

			for _, want := range tt.wantContain {
				if !strings.Contains(dockerfile, want) {
					t.Errorf("Dockerfile missing %q\ngot:\n%s", want, dockerfile)
				}
			}
		})
	}
}

func TestGenerateWrapperConfig(t *testing.T) {
	tests := []struct {
		name        string
		opts        BuildOptions
		wantContain []string
	}{
		{
			name: "amd64 Lyra",
			opts: BuildOptions{
				ServerPort:  7777,
				ProjectName: "Lyra",
			},
			wantContain: []string{
				"gamePort: 7777",
				"./Lyra/Binaries/Linux/LyraServer",
				"\"Lyra\"",
			},
		},
		{
			name: "arm64 custom",
			opts: BuildOptions{
				ServerPort:   8888,
				ProjectName:  "FPS",
				ServerTarget: "FPSServer",
				Arch:         "arm64",
			},
			wantContain: []string{
				"gamePort: 8888",
				"./FPS/Binaries/LinuxArm64/FPSServer",
				"\"FPS\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(tt.opts, nil)
			config := b.GenerateWrapperConfig()

			for _, want := range tt.wantContain {
				if !strings.Contains(config, want) {
					t.Errorf("wrapper config missing %q\ngot:\n%s", want, config)
				}
			}
		})
	}
}

func TestGenerateDockerignore(t *testing.T) {
	b := NewBuilder(BuildOptions{}, nil)
	ignore := b.GenerateDockerignore()

	wantPatterns := []string{"**/*.debug", "**/*.sym", "Manifest_*.txt"}
	for _, p := range wantPatterns {
		if !strings.Contains(ignore, p) {
			t.Errorf("dockerignore missing pattern %q", p)
		}
	}
}

func TestResolveServerBinaryName(t *testing.T) {
	t.Run("no build dir", func(t *testing.T) {
		b := NewBuilder(BuildOptions{
			ProjectName:  "Lyra",
			ServerTarget: "LyraServer",
		}, nil)
		got := b.resolveServerBinaryName()
		if got != "LyraServer" {
			t.Errorf("got %q, want %q", got, "LyraServer")
		}
	})

	t.Run("development build", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "Lyra", "Binaries", "Linux")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatal(err)
		}
		// Development build: bare target name
		if err := os.WriteFile(filepath.Join(binDir, "LyraServer"), []byte("binary"), 0755); err != nil {
			t.Fatal(err)
		}

		b := NewBuilder(BuildOptions{
			ServerBuildDir: tmpDir,
			ProjectName:    "Lyra",
			ServerTarget:   "LyraServer",
		}, nil)
		got := b.resolveServerBinaryName()
		if got != "LyraServer" {
			t.Errorf("got %q, want %q", got, "LyraServer")
		}
	})

	t.Run("shipping build", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "Lyra", "Binaries", "Linux")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatal(err)
		}
		// Shipping build: Target-Platform-Config pattern
		if err := os.WriteFile(filepath.Join(binDir, "LyraServer-Linux-Shipping"), []byte("binary"), 0755); err != nil {
			t.Fatal(err)
		}

		b := NewBuilder(BuildOptions{
			ServerBuildDir: tmpDir,
			ProjectName:    "Lyra",
			ServerTarget:   "LyraServer",
		}, nil)
		got := b.resolveServerBinaryName()
		if got != "LyraServer-Linux-Shipping" {
			t.Errorf("got %q, want %q", got, "LyraServer-Linux-Shipping")
		}
	})

	t.Run("arm64 shipping build", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "Lyra", "Binaries", "LinuxArm64")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "LyraServer-LinuxArm64-Shipping"), []byte("binary"), 0755); err != nil {
			t.Fatal(err)
		}

		b := NewBuilder(BuildOptions{
			ServerBuildDir: tmpDir,
			ProjectName:    "Lyra",
			ServerTarget:   "LyraServer",
			Arch:           "arm64",
		}, nil)
		got := b.resolveServerBinaryName()
		if got != "LyraServer-LinuxArm64-Shipping" {
			t.Errorf("got %q, want %q", got, "LyraServer-LinuxArm64-Shipping")
		}
	})
}
