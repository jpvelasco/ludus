package dockerbuild

import (
	"bytes"
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
			// #271: the container path follows the actual .uproject filename from the
			// path even when projectName is unset; projectName only defaults (to Lyra)
			// for target names, not for the on-disk uproject filename.
			name: "external project derives filename from path basename",
			opts: DockerGameOptions{ProjectPath: "/home/user/LyraStarterGame/LyraStarterGame.uproject"},
			want: "/project/LyraStarterGame.uproject",
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

// TestBuildResultForArm64 and amd64 (dry-run) to verify correct output dirs/binaries.
func TestBuildResultForArm64(t *testing.T) {
	tests := []struct {
		name   string
		arch   string
		wantIn string
	}{
		{"arm64", "arm64", "LinuxArm64Server"},
		{"amd64", "amd64", "LinuxServer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DockerGameOptions{EngineImage: "test:tag", ProjectName: "Lyra", Arch: tt.arch}

			// Capture runner output to assert container platform force (always linux/amd64
			// for game builds via container, even arm64; arm64 is cross inside via UAT).
			// Covers "via container" + no regression on amd64.
			r := runner.NewRunner(false, true)
			var buf bytes.Buffer
			r.Stdout = &buf

			b := NewDockerGameBuilder(opts, r)
			res, err := b.Build(context.Background())
			if err != nil {
				t.Fatalf("Build err: %v", err)
			}
			if !strings.Contains(res.OutputDir, tt.wantIn) || !strings.Contains(res.ServerBinary, tt.wantIn) {
				t.Errorf("for arch=%s got Output=%s Binary=%s want %s", tt.arch, res.OutputDir, res.ServerBinary, tt.wantIn)
			}
			if !res.Success {
				t.Error("expected Success")
			}

			out := buf.String()
			if !strings.Contains(out, "--platform linux/amd64") {
				t.Errorf("expected echoed docker run to contain --platform linux/amd64 (forced for container game builds), got: %s", out)
			}
		})
	}
}

// TestBuildClientResultForArm64 covers arm64/amd64 client output (dry-run).
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
