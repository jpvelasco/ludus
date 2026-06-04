package dockerbuild

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

func TestNewDockerGameBuilder(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name         string
		opts         DockerGameOptions
		wantPlatform string
	}{
		{
			name:         "default platform is Linux",
			opts:         DockerGameOptions{},
			wantPlatform: "Linux",
		},
		{
			name:         "explicit platform preserved",
			opts:         DockerGameOptions{Platform: "Win64"},
			wantPlatform: "Win64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			if b.opts.Platform != tt.wantPlatform {
				t.Errorf("Platform = %q, want %q", b.opts.Platform, tt.wantPlatform)
			}
		})
	}
}

func TestResolveProjectName(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name string
		opts DockerGameOptions
		want string
	}{
		{
			name: "defaults to Lyra",
			opts: DockerGameOptions{},
			want: "Lyra",
		},
		{
			name: "custom project name",
			opts: DockerGameOptions{ProjectName: "ShooterGame"},
			want: "ShooterGame",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.resolveProjectName()
			if got != tt.want {
				t.Errorf("resolveProjectName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveServerTarget(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name string
		opts DockerGameOptions
		want string
	}{
		{
			name: "defaults to LyraServer",
			opts: DockerGameOptions{},
			want: "LyraServer",
		},
		{
			name: "custom project derives target",
			opts: DockerGameOptions{ProjectName: "ShooterGame"},
			want: "ShooterGameServer",
		},
		{
			name: "explicit server target",
			opts: DockerGameOptions{ServerTarget: "MyCustomServer"},
			want: "MyCustomServer",
		},
		{
			name: "explicit target overrides project name derivation",
			opts: DockerGameOptions{ProjectName: "ShooterGame", ServerTarget: "SGServer"},
			want: "SGServer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.resolveServerTarget()
			if got != tt.want {
				t.Errorf("resolveServerTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveGameTarget(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name string
		opts DockerGameOptions
		want string
	}{
		{
			name: "defaults to LyraGame",
			opts: DockerGameOptions{},
			want: "LyraGame",
		},
		{
			name: "custom project derives target",
			opts: DockerGameOptions{ProjectName: "ShooterGame"},
			want: "ShooterGameGame",
		},
		{
			name: "explicit game target",
			opts: DockerGameOptions{GameTarget: "MyGameTarget"},
			want: "MyGameTarget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.resolveGameTarget()
			if got != tt.want {
				t.Errorf("resolveGameTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsExternalProject(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name string
		opts DockerGameOptions
		want bool
	}{
		{
			name: "no project path is not external",
			opts: DockerGameOptions{},
			want: false,
		},
		{
			name: "with project path is external",
			opts: DockerGameOptions{ProjectPath: "/home/user/MyGame/MyGame.uproject"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.isExternalProject()
			if got != tt.want {
				t.Errorf("isExternalProject() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainerProjectPath(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name string
		opts DockerGameOptions
		want string
	}{
		{
			name: "in-engine Lyra default",
			opts: DockerGameOptions{},
			want: "/engine/Samples/Games/Lyra/Lyra.uproject",
		},
		{
			name: "in-engine custom project",
			opts: DockerGameOptions{ProjectName: "ShooterGame"},
			want: "/engine/Samples/Games/ShooterGame/ShooterGame.uproject",
		},
		{
			name: "external project",
			opts: DockerGameOptions{ProjectPath: "/home/user/MyGame/MyGame.uproject", ProjectName: "MyGame"},
			want: "/project/MyGame.uproject",
		},
		{
			name: "external project defaults name to Lyra",
			opts: DockerGameOptions{ProjectPath: "/some/path"},
			want: "/project/Lyra.uproject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.containerProjectPath()
			if got != tt.want {
				t.Errorf("containerProjectPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveClientPlatform(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name string
		opts DockerGameOptions
		want string
	}{
		{
			name: "defaults to Linux",
			opts: DockerGameOptions{},
			want: "Linux",
		},
		{
			name: "explicit platform",
			opts: DockerGameOptions{ClientPlatform: "Win64"},
			want: "Win64",
		},
		{
			name: "arm64 arch derives LinuxArm64",
			opts: DockerGameOptions{Arch: "arm64"},
			want: "LinuxArm64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.resolveClientPlatform()
			if got != tt.want {
				t.Errorf("resolveClientPlatform() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveArch(t *testing.T) {
	r := runner.NewRunner(false, false)
	tests := []struct {
		name string
		opts DockerGameOptions
		want string
	}{
		{"default", DockerGameOptions{}, "amd64"},
		{"arm64", DockerGameOptions{Arch: "arm64"}, "arm64"},
		{"aarch64", DockerGameOptions{Arch: "aarch64"}, "arm64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			if got := b.resolveArch(); got != tt.want {
				t.Errorf("resolveArch() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBuildResultForArm64 exercises the full DockerGameBuilder.Build path (dry-run)
// to verify arm64 produces LinuxArm64Server output dir + binary, and amd64 has no regression.
func TestBuildResultForArm64(t *testing.T) {
	r := runner.NewRunner(false, true) // dry-run to exercise result population without Docker

	tests := []struct {
		name         string
		arch         string
		wantPlatDir  string
		wantInOutput string
		wantInBinary string
	}{
		{
			name:         "arm64",
			arch:         "arm64",
			wantPlatDir:  "LinuxArm64Server",
			wantInOutput: "LinuxArm64Server",
			wantInBinary: "LinuxArm64Server",
		},
		{
			name:         "amd64 default no regression",
			arch:         "",
			wantPlatDir:  "LinuxServer",
			wantInOutput: "LinuxServer",
			wantInBinary: "LinuxServer",
		},
		{
			name:         "amd64 explicit",
			arch:         "amd64",
			wantPlatDir:  "LinuxServer",
			wantInOutput: "LinuxServer",
			wantInBinary: "LinuxServer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DockerGameOptions{
				EngineImage: "ludus-engine:5.6-test",
				ProjectName: "Lyra",
				Arch:        tt.arch,
			}
			b := NewDockerGameBuilder(opts, r)
			res, err := b.Build(context.Background())
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if !strings.Contains(res.OutputDir, tt.wantInOutput) {
				t.Errorf("OutputDir %q should contain %q for arch=%q", res.OutputDir, tt.wantInOutput, tt.arch)
			}
			if !strings.Contains(res.ServerBinary, tt.wantInBinary) {
				t.Errorf("ServerBinary %q should contain %q for arch=%q", res.ServerBinary, tt.wantInBinary, tt.arch)
			}
			if res.Success != true {
				t.Error("expected Success=true in dry-run result")
			}
		})
	}
}

// TestBuildClientResultForArm64 exercises DockerGameBuilder.BuildClient for arm64/amd64 output paths.
func TestBuildClientResultForArm64(t *testing.T) {
	r := runner.NewRunner(false, true)

	tests := []struct {
		name         string
		arch         string
		wantInBinary string
	}{
		{name: "arm64 client", arch: "arm64", wantInBinary: "LinuxArm64"},
		{name: "amd64 client", arch: "amd64", wantInBinary: "Linux"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DockerGameOptions{
				EngineImage: "ludus-engine:5.6-test",
				ProjectName: "Lyra",
				Arch:        tt.arch,
			}
			b := NewDockerGameBuilder(opts, r)
			res, err := b.BuildClient(context.Background())
			if err != nil {
				t.Fatalf("BuildClient() error = %v", err)
			}
			if !strings.Contains(res.ClientBinary, tt.wantInBinary) {
				t.Errorf("ClientBinary %q should contain %q for arch=%q", res.ClientBinary, tt.wantInBinary, tt.arch)
			}
		})
	}
}
