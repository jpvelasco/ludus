package dockerbuild

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

var generateBuildScriptServerTests = []struct {
	name        string
	opts        DockerGameOptions
	contains    []string
	notContains []string
}{
	{
		name: "default Lyra server build",
		opts: DockerGameOptions{},
		contains: []string{
			"#!/bin/bash", "set -e", "RunUAT.sh BuildCookRun",
			"-server -noclient", "-servertargetname=LyraServer",
			"-build -stage -package -archive", `-archivedirectory="/output"`,
			"-cook", "Lyra/Lyra.uproject", "DefaultServerTarget",
		},
		notContains: []string{"-skipcook"},
	},
	{
		name:        "arm64 server",
		opts:        DockerGameOptions{Arch: "arm64"},
		contains:    []string{"-platform=Linux", "-serverplatform=Linux.LinuxArm64"},
		notContains: []string{"TargetArchitecture=AArch64"},
	},
	{
		name:        "skip cook",
		opts:        DockerGameOptions{SkipCook: true},
		contains:    []string{"-skipcook"},
		notContains: []string{"  -cook"},
	},
	{
		name:     "with server map",
		opts:     DockerGameOptions{ServerMap: "MyMap"},
		contains: []string{`-map="MyMap"`},
	},
	{
		name:        "no map by default",
		opts:        DockerGameOptions{},
		notContains: []string{"-map="},
	},
	{
		name:     "custom project and target",
		opts:     DockerGameOptions{ProjectName: "ShooterGame", ServerTarget: "SGServer"},
		contains: []string{"-servertargetname=SGServer", "ShooterGame/ShooterGame.uproject"},
	},
	{
		name: "external project",
		opts: DockerGameOptions{
			ProjectPath: "/home/user/MyGame/MyGame.uproject", ProjectName: "MyGame", ServerTarget: "MyGameServer",
		},
		contains: []string{"/project/MyGame.uproject", "-servertargetname=MyGameServer"},
	},
	{
		// #271: uproject filename differs from projectName. Epic ships the Lyra
		// sample as LyraStarterGame.uproject but its targets are LyraServer/LyraGame,
		// so projectName is set to "Lyra". The container path must follow the actual
		// filename (mounted at /project), not <projectName>.uproject.
		name: "external project with filename != projectName",
		opts: DockerGameOptions{
			ProjectPath: "/home/user/LyraStarterGame/LyraStarterGame.uproject", ProjectName: "Lyra", ServerTarget: "LyraServer",
		},
		contains:    []string{"/project/LyraStarterGame.uproject", "-servertargetname=LyraServer"},
		notContains: []string{"/project/Lyra.uproject"},
	},
}

func TestGenerateBuildScript_Server(t *testing.T) {
	r := runner.NewRunner(false, false)

	for _, tt := range generateBuildScriptServerTests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.generateBuildScript(true)

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("server script should contain %q\ngot:\n%s", want, got)
				}
			}
			for _, notWant := range tt.notContains {
				if strings.Contains(got, notWant) {
					t.Errorf("server script should not contain %q\ngot:\n%s", notWant, got)
				}
			}
		})
	}
}

var generateBuildScriptClientTests = []struct {
	name        string
	opts        DockerGameOptions
	contains    []string
	notContains []string
}{
	{
		name: "default client build",
		opts: DockerGameOptions{},
		contains: []string{
			"#!/bin/bash",
			"set -e",
			"RunUAT.sh BuildCookRun",
			"-platform=Linux",
			"-build -stage -package -archive",
			`-archivedirectory="/output"`,
			"-cook",
			"Lyra/Lyra.uproject",
		},
		notContains: []string{
			"-server",
			"-noclient",
			"-servertargetname",
			"-skipcook",
		},
	},
	{
		name:     "custom client platform",
		opts:     DockerGameOptions{ClientPlatform: "Win64"},
		contains: []string{"-platform=Win64"},
	},
	{
		name:        "skip cook client",
		opts:        DockerGameOptions{SkipCook: true},
		contains:    []string{"-skipcook"},
		notContains: []string{"  -cook"},
	},
	{
		name: "external project client",
		opts: DockerGameOptions{
			ProjectPath: "/home/user/MyGame/MyGame.uproject",
			ProjectName: "MyGame",
		},
		contains: []string{"/project/MyGame.uproject"},
	},
	{
		name: "external project client with filename != projectName",
		opts: DockerGameOptions{
			ProjectPath: "/home/user/LyraStarterGame/LyraStarterGame.uproject",
			ProjectName: "Lyra",
		},
		contains:    []string{"/project/LyraStarterGame.uproject"},
		notContains: []string{"/project/Lyra.uproject"},
	},
	{
		name:     "arm64 client",
		opts:     DockerGameOptions{Arch: "arm64"},
		contains: []string{"-platform=LinuxArm64"},
	},
}

func TestGenerateBuildScript_Client(t *testing.T) {
	r := runner.NewRunner(false, false)

	for _, tt := range generateBuildScriptClientTests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.generateBuildScript(false)

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("client script should contain %q\ngot:\n%s", want, got)
				}
			}
			for _, notWant := range tt.notContains {
				if strings.Contains(got, notWant) {
					t.Errorf("client script should not contain %q\ngot:\n%s", notWant, got)
				}
			}
		})
	}
}

func TestGenerateBuildScript_ServerContainsCdEngine(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{}, r)
	got := b.generateBuildScript(true)
	if !strings.Contains(got, "cd /engine") {
		t.Errorf("server build script should contain 'cd /engine'\ngot:\n%s", got)
	}
}

func TestGenerateBuildScript_ClientContainsCdEngine(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{}, r)
	got := b.generateBuildScript(false)
	if !strings.Contains(got, "cd /engine") {
		t.Errorf("client build script should contain 'cd /engine'\ngot:\n%s", got)
	}
}

func TestGenerateBuildScript_CookOnly(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{CookOnly: true, DDCMode: "local", DDCPath: "/tmp/ddc"}, r)
	got := b.generateBuildScript(true)

	mustContain := []string{
		"-cook",
		"-skipbuild",
		"-NoCompileEditor -NoP4",
		"-server -noclient",
		"-map=MinimalDefaultMap",
	}
	mustNotContain := []string{
		"-archivedirectory",
		"DefaultServerTarget",
		"-servertargetname",
		"-NoCompile ",
	}

	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Errorf("cook-only script should contain %q\ngot:\n%s", want, got)
		}
	}
	for _, notWant := range mustNotContain {
		if strings.Contains(got, notWant) {
			t.Errorf("cook-only script should not contain %q\ngot:\n%s", notWant, got)
		}
	}
}
