package container

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
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
		{
			// Real-world case that broke the live build: the .uproject name
			// (packaged content dir), the project name, and the server target
			// are all different. The Dockerfile must chmod the binary under the
			// PACKAGED DIR name, not ProjectName.
			name: "packaged dir differs from project name and target",
			opts: BuildOptions{
				ServerPort:      7777,
				ProjectName:     "Lyra",
				PackagedDirName: "LyraStarterGame6",
				ServerTarget:    "LyraServer",
			},
			wantContain: []string{
				"LyraStarterGame6/Binaries/Linux/LyraServer",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(tt.opts, nil)
			dockerfile := b.GenerateDockerfile()
			// Guard: when a distinct packaged dir is set, the bare project name
			// must NOT appear as a content-dir path segment.
			if tt.opts.PackagedDirName != "" && tt.opts.PackagedDirName != tt.opts.ProjectName {
				if strings.Contains(dockerfile, "/"+tt.opts.ProjectName+"/Binaries") ||
					strings.Contains(dockerfile, " "+tt.opts.ProjectName+"/Binaries") {
					t.Errorf("Dockerfile used ProjectName %q for content dir instead of PackagedDirName %q\ngot:\n%s",
						tt.opts.ProjectName, tt.opts.PackagedDirName, dockerfile)
				}
			}

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
		{
			// Wrapper config must use the packaged dir name for BOTH the exec
			// path and the project argument passed to the server binary (UE's
			// generated <Target>.sh passes the .uproject name, not ProjectName).
			name: "packaged dir differs from project name",
			opts: BuildOptions{
				ServerPort:      7777,
				ProjectName:     "Lyra",
				PackagedDirName: "LyraStarterGame6",
				ServerTarget:    "LyraServer",
			},
			wantContain: []string{
				"./LyraStarterGame6/Binaries/Linux/LyraServer",
				"arg: \"LyraStarterGame6\"",
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

func TestResolveCLI(t *testing.T) {
	tests := []struct {
		name    string
		backend string
		want    string
	}{
		{"podman backend returns podman", "podman", "podman"},
		{"docker backend returns docker", "docker", "docker"},
		{"empty backend returns docker", "", "docker"},
		{"unrecognised backend returns docker", "wsl2", "docker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(BuildOptions{Backend: tt.backend}, nil)
			got := b.resolveCLI()
			if got != tt.want {
				t.Errorf("resolveCLI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunDockerBuild_ProvenanceFlag(t *testing.T) {
	tests := []struct {
		name           string
		backend        string
		wantProvenance bool // true = expect --provenance=false in output
	}{
		{"docker includes provenance flag", "docker", true},
		{"podman omits provenance flag", "podman", false},
		{"empty backend (defaults to docker) includes provenance flag", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout strings.Builder
			r := runner.NewRunner(false, true) // dry-run: prints args, doesn't exec
			r.Stdout = &stdout

			b := NewBuilder(BuildOptions{
				Backend:        tt.backend,
				ServerBuildDir: t.TempDir(),
				ImageName:      "test-image",
				Tag:            "latest",
				Arch:           runtime.GOARCH, // match host arch to skip cross-arch check
			}, r)

			ctx := context.Background()
			_ = b.runDockerBuild(ctx, "test-image:latest")

			output := stdout.String()
			hasProvenance := strings.Contains(output, "--provenance=false")
			if hasProvenance != tt.wantProvenance {
				if tt.wantProvenance {
					t.Errorf("expected --provenance=false in dry-run output, got: %s", output)
				} else {
					t.Errorf("unexpected --provenance=false in dry-run output for podman, got: %s", output)
				}
			}
		})
	}
}

// TestBuild_DryRun verifies that container build under --dry-run succeeds cleanly
// (no attempt to ensure wrapper / read template / stage files into ServerBuildDir).
// It is the first test to exercise full Builder.Build() with a dry-run runner.
// Previously this path would fail with "reading config template" on cache miss
// (Windows cross-compile path, or any host without pre-cached wrapper).
func TestBuild_DryRun(t *testing.T) {
	var stdout strings.Builder
	r := runner.NewRunner(false, true) // dry-run: prints args, doesn't exec
	r.Stdout = &stdout

	b := NewBuilder(BuildOptions{
		ServerBuildDir: t.TempDir(),
		ImageName:      "test-image",
		Tag:            "latest",
		ServerPort:     7777,
		Arch:           runtime.GOARCH, // match host arch to skip cross-arch check
	}, r)

	ctx := context.Background()
	res, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build() under dry-run should succeed cleanly, got: %v", err)
	}
	if res == nil {
		t.Fatal("Build() returned nil result under dry-run")
	}
	if !res.Success {
		t.Error("expected res.Success == true under dry-run")
	}

	output := stdout.String()
	if !strings.Contains(output, "+ docker build") {
		t.Errorf("expected echoed docker build cmd in dry-run output, got: %s", output)
	}
	if strings.Contains(output, "reading config template") || strings.Contains(output, "game server wrapper") {
		t.Errorf("dry-run should not attempt wrapper/template load, got in output: %s", output)
	}
}
