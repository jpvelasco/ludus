package container

import (
	"strings"
	"testing"
)

func TestNewBuilder(t *testing.T) {
	tests := []struct {
		name string
		opts BuildOptions
	}{
		{
			name: "zero value opts",
			opts: BuildOptions{},
		},
		{
			name: "fully populated opts",
			opts: BuildOptions{
				ServerBuildDir: "/tmp/server",
				ImageName:      "my-game",
				Tag:            "latest",
				ServerPort:     7777,
				NoCache:        true,
				ProjectName:    "Lyra",
				ServerTarget:   "LyraServer",
				Arch:           "arm64",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(tt.opts, nil)
			if b == nil {
				t.Fatal("NewBuilder returned nil")
			}
			if b.opts != tt.opts {
				t.Errorf("opts mismatch: got %+v, want %+v", b.opts, tt.opts)
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
