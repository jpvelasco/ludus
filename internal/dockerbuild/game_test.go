package dockerbuild

import (
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
)

func TestScriptPreamble(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{}, r)
	got := b.scriptPreamble()

	assertContains(t, got, []string{
		"#!/bin/bash",
		"set -e",
		"useradd",           // UE 5.7+ refuses root on x86_64
		"su -p ue",          // preserve env when switching user
		"bash /build.sh",    // exec as ue user
		"HOME=/home/ue",     // su -p keeps HOME=/root
		"Build/Scripts/obj", // AutomationTool C# compilation
		"*.sym",             // pre-built .sym files for linker
	})

	// NuGet workaround is NOT in the preamble (moved to container -e args)
	if strings.Contains(got, "NuGetAuditLevel") {
		t.Error("NuGet workaround should not be in preamble (use envArgs instead)")
	}
}

func TestEnvArgs(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name      string
		opts      DockerGameOptions
		wantNuGet bool
	}{
		{
			name:      "empty version gets NuGet env",
			opts:      DockerGameOptions{},
			wantNuGet: true,
		},
		{
			name:      "5.6 gets NuGet env",
			opts:      DockerGameOptions{EngineVersion: "5.6"},
			wantNuGet: true,
		},
		{
			name:      "5.7 skips NuGet env",
			opts:      DockerGameOptions{EngineVersion: "5.7"},
			wantNuGet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			args := b.envArgs()
			hasNuGet := false
			for _, a := range args {
				if strings.Contains(a, "NuGetAuditLevel") {
					hasNuGet = true
				}
			}
			if hasNuGet != tt.wantNuGet {
				t.Errorf("NuGet env arg present = %v, want %v; args = %v", hasNuGet, tt.wantNuGet, args)
			}
		})
	}
}

func TestScriptPreamble_InstallsRuntimeDeps(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{EngineVersion: "5.7"}, r)
	got := b.scriptPreamble()

	if !strings.Contains(got, "ldconfig") {
		t.Error("preamble should use ldconfig to check for missing libs")
	}

	// The preamble must include every package from AptRuntimePackages (single source of truth).
	for _, pkg := range AptRuntimePackages {
		if !strings.Contains(got, pkg) {
			t.Errorf("preamble should install %q for UnrealEditor-Cmd runtime deps", pkg)
		}
	}

	// The preamble must fail fast if apt-get install fails, not silently continue
	// through a multi-hour compile only to crash at cook.
	if !strings.Contains(got, "exit 1") {
		t.Error("preamble must fail fast on install failure (exit 1)")
	}
}

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
