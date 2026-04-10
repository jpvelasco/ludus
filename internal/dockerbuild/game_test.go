package dockerbuild

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
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

func TestScriptPreamble(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{}, r)
	got := b.scriptPreamble()

	if !strings.Contains(got, "#!/bin/bash") {
		t.Error("preamble should contain #!/bin/bash")
	}
	if !strings.Contains(got, "set -e") {
		t.Error("preamble should contain set -e")
	}

	// Preamble must create non-root user (UE 5.7+ refuses root on x86_64)
	if !strings.Contains(got, "useradd") {
		t.Error("preamble should create a non-root user with useradd")
	}
	if !strings.Contains(got, "su -p ue") {
		t.Error("preamble should switch to ue user with su -p (preserving env)")
	}
	if !strings.Contains(got, "bash /build.sh") {
		t.Error("preamble should exec the build script as the ue user")
	}
	if !strings.Contains(got, "HOME=/home/ue") {
		t.Error("preamble should override HOME for the ue user (su -p keeps HOME=/root)")
	}

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

func TestDDCArgs(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name       string
		ddcMode    string
		ddcPath    string
		wantErr    string
		wantArgs   []string
		wantNoArgs bool
	}{
		{
			name:    "local mode with empty path errors",
			ddcMode: "local",
			ddcPath: "",
			wantErr: "no path configured",
		},
		{
			name:    "unsupported mode errors",
			ddcMode: "s3",
			ddcPath: "/some/path",
			wantErr: "unsupported DDC mode",
		},
		{
			name:       "none mode returns no args",
			ddcMode:    "none",
			wantNoArgs: true,
		},
		{
			name:       "empty mode returns no args",
			ddcMode:    "",
			wantNoArgs: true,
		},
		{
			name:    "local mode returns volume and env args",
			ddcMode: "local",
			ddcPath: filepath.Join(t.TempDir(), "ddc"),
			wantArgs: []string{
				"-v", // volume mount present
				"/ddc",
				"-e",
				"UE-LocalDataCachePath=/ddc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(DockerGameOptions{
				DDCMode: tt.ddcMode,
				DDCPath: tt.ddcPath,
			}, r)

			args, err := b.ddcArgs()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error should contain %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNoArgs {
				if len(args) != 0 {
					t.Errorf("expected no args, got %v", args)
				}
				return
			}

			joined := strings.Join(args, " ")
			for _, want := range tt.wantArgs {
				if !strings.Contains(joined, want) {
					t.Errorf("args should contain %q, got: %v", want, args)
				}
			}
		})
	}
}

func TestDDCArgs_LocalCreatesDirectory(t *testing.T) {
	ddcDir := filepath.Join(t.TempDir(), "ddc")
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{
		DDCMode: "local",
		DDCPath: ddcDir,
	}, r)

	_, err := b.ddcArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(ddcDir); err != nil {
		t.Errorf("DDC directory should have been created: %v", err)
	}
}

func TestDDCArgs_LocalVolumeFormat(t *testing.T) {
	ddcDir := filepath.Join(t.TempDir(), "ddc")
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{
		DDCMode: "local",
		DDCPath: ddcDir,
	}, r)

	args, err := b.ddcArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect exactly 4 args: -v <host>:/ddc -e UE-LocalDataCachePath=/ddc
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d: %v", len(args), args)
	}
	if args[0] != "-v" {
		t.Errorf("args[0] = %q, want -v", args[0])
	}
	if !strings.HasSuffix(args[1], ":/ddc") {
		t.Errorf("args[1] = %q, should end with :/ddc", args[1])
	}
	if args[2] != "-e" {
		t.Errorf("args[2] = %q, want -e", args[2])
	}
	if args[3] != "UE-LocalDataCachePath=/ddc" {
		t.Errorf("args[3] = %q, want UE-LocalDataCachePath=/ddc", args[3])
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

func TestDDCNotInBuildScript(t *testing.T) {
	tests := []struct {
		name string
		opts DockerGameOptions
	}{
		{
			name: "local mode does not embed DDC args in script",
			opts: DockerGameOptions{DDCMode: "local", DDCPath: "/tmp/ddc"},
		},
		{
			name: "none mode has no DDC args in script",
			opts: DockerGameOptions{DDCMode: "none"},
		},
		{
			name: "empty mode has no DDC args in script",
			opts: DockerGameOptions{},
		},
	}

	notExpected := []string{"-ini:Engine:[DerivedDataBackendGraph]", "UE-LocalDataCachePath"}

	r := runner.NewRunner(false, false)
	for _, tt := range tests {
		t.Run(tt.name+" (server)", func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.generateBuildScript(true)
			for _, notWant := range notExpected {
				if strings.Contains(got, notWant) {
					t.Errorf("server script should NOT contain %q\ngot:\n%s", notWant, got)
				}
			}
		})
		t.Run(tt.name+" (client)", func(t *testing.T) {
			b := NewDockerGameBuilder(tt.opts, r)
			got := b.generateBuildScript(false)
			for _, notWant := range notExpected {
				if strings.Contains(got, notWant) {
					t.Errorf("client script should NOT contain %q\ngot:\n%s", notWant, got)
				}
			}
		})
	}
}

func TestGenerateBuildScript_CookOnly(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewDockerGameBuilder(DockerGameOptions{CookOnly: true, DDCMode: "local", DDCPath: "/tmp/ddc"}, r)
	got := b.generateBuildScript(true)

	mustContain := []string{
		"-cook",
		"-skipbuild",
		"-NoCompile -NoCompileEditor -NoP4",
		"-server -noclient",
		"-map=MinimalDefaultMap",
	}
	mustNotContain := []string{
		"-archivedirectory",
		"DefaultServerTarget",
		"-servertargetname",
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
