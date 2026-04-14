package wsl

import (
	"slices"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/ddc"
)

func TestValidateBuildGameOpts(t *testing.T) {
	tests := []struct {
		name    string
		opts    GameOptions
		wantErr string // substring of expected error message; "" = no error
	}{
		{
			name:    "empty EnginePath errors with actionable message",
			opts:    GameOptions{},
			wantErr: "ludus engine build --backend wsl2",
		},
		{
			// local mode with no path must error — the build would silently
			// proceed without a DDC cache, making the issue hard to diagnose.
			name:    "local DDC mode with empty DDCPath errors",
			opts:    GameOptions{EnginePath: "/opt/ue/5.7", DDCMode: ddc.ModeLocal, DDCPath: ""},
			wantErr: "ddc.path in ludus.yaml",
		},
		{
			// none mode never requires a path — the cache is simply disabled.
			name: "none DDC mode with empty DDCPath is valid",
			opts: GameOptions{EnginePath: "/opt/ue/5.7", DDCMode: ddc.ModeNone, DDCPath: ""},
		},
		{
			name: "local DDC mode with path set is valid",
			opts: GameOptions{EnginePath: "/opt/ue/5.7", DDCMode: ddc.ModeLocal, DDCPath: "/home/user/ddc"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBuildGameOpts(tt.opts)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestBuildGameScript_DDCLocal(t *testing.T) {
	const proj = "/mnt/f/game/MyGame.uproject"
	const out = "/mnt/f/out"

	tests := []struct {
		name           string
		opts           GameOptions
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:           "uses env not export",
			opts:           GameOptions{EnginePath: "/opt/ue/5.7", DDCMode: ddc.ModeLocal, DDCPath: "/home/user/ddc"},
			wantContains:   []string{`env 'UE-LocalDataCachePath=`},
			wantNotContain: []string{"export UE-LocalDataCachePath", "export UE-"},
		},
		{
			name: "pre-resolved absolute paths are single-quoted",
			opts: GameOptions{EnginePath: "/home/user/ludus/engine/5.7", DDCMode: ddc.ModeLocal, DDCPath: "/home/user/ludus/ddc"},
			wantContains: []string{
				`cd '/home/user/ludus/engine/5.7'`,
				`env 'UE-LocalDataCachePath=/home/user/ludus/ddc'`,
			},
		},
		{
			name:         "path with spaces preserved in env arg",
			opts:         GameOptions{EnginePath: "/opt/ue/5.7", DDCMode: ddc.ModeLocal, DDCPath: "/home/user/my ddc"},
			wantContains: []string{`env 'UE-LocalDataCachePath=/home/user/my ddc'`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertGameScript(t, tt.opts, proj, out, tt.wantContains, tt.wantNotContain)
		})
	}
}

func TestBuildGameScript_DDCDisabled(t *testing.T) {
	tests := []struct {
		name           string
		opts           GameOptions
		wantNotContain []string
	}{
		{
			name:           "DDC none: no env prefix",
			opts:           GameOptions{EnginePath: "/opt/ue/5.7", DDCMode: ddc.ModeNone},
			wantNotContain: []string{"UE-LocalDataCachePath", "env "},
		},
		{
			name:           "DDC local with empty path: no env prefix",
			opts:           GameOptions{EnginePath: "/opt/ue/5.7", DDCMode: ddc.ModeLocal, DDCPath: ""},
			wantNotContain: []string{"UE-LocalDataCachePath"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertGameScript(t, tt.opts, "/mnt/f/game/MyGame.uproject", "/mnt/f/out", nil, tt.wantNotContain)
		})
	}
}

func assertGameScript(t *testing.T, opts GameOptions, projectPath, outputDir string, wantContains, wantNotContain []string) {
	t.Helper()
	script := buildGameScript(opts, projectPath, outputDir)
	for _, want := range wantContains {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q\ngot: %s", want, script)
		}
	}
	for _, notWant := range wantNotContain {
		if strings.Contains(script, notWant) {
			t.Errorf("script should not contain %q\ngot: %s", notWant, script)
		}
	}
}

func TestBuildRunUATArgs_Basics(t *testing.T) {
	tests := []struct {
		name        string
		opts        GameOptions
		projectPath string
		outputDir   string
		wantContain []string
	}{
		{
			"basic server build",
			GameOptions{Platform: "Linux", ServerConfig: "Development"},
			"/mnt/f/game/MyGame.uproject", "/mnt/f/game/PackagedServer",
			[]string{"BuildCookRun", "-project='/mnt/f/game/MyGame.uproject'",
				"-platform=Linux", "-server", "-noclient", "-cook", "-serverconfig=Development"},
		},
		{
			"defaults to Linux and Development",
			GameOptions{}, "/proj", "/out",
			[]string{"-platform=Linux", "-serverconfig=Development"},
		},
		{
			"with server target and map",
			GameOptions{ServerTarget: "LyraServer", ServerMap: "/Game/Maps/Expanse"},
			"/mnt/f/game/Lyra.uproject", "/mnt/f/out",
			[]string{"-target=LyraServer", "-map=/Game/Maps/Expanse"},
		},
		{
			"with max jobs",
			GameOptions{MaxJobs: 8}, "/mnt/f/game/My.uproject", "/mnt/f/out",
			[]string{"-MaxParallelActions=8"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRunUATArgs(t, tt.opts, tt.projectPath, tt.outputDir, tt.wantContain, nil)
		})
	}
}

func TestBuildRunUATArgs_SkipCookAndQuoting(t *testing.T) {
	t.Run("skip cook", func(t *testing.T) {
		assertRunUATArgs(t,
			GameOptions{SkipCook: true}, "/mnt/f/game/My.uproject", "/mnt/f/out",
			[]string{"-skipcook"}, []string{"-cook"})
	})

	t.Run("path with spaces: project and archive are single-quoted", func(t *testing.T) {
		assertRunUATArgs(t,
			GameOptions{Platform: "Linux"},
			"/mnt/f/Source Code/MyGame.uproject", "/mnt/f/Source Code/Packaged",
			[]string{
				"-project='/mnt/f/Source Code/MyGame.uproject'",
				"-archivedirectory='/mnt/f/Source Code/Packaged'",
			}, nil)
	})
}

func assertRunUATArgs(t *testing.T, opts GameOptions, projectPath, outputDir string, wantContain, wantExclude []string) {
	t.Helper()
	args := buildRunUATArgs(opts, projectPath, outputDir)
	joined := strings.Join(args, " ")
	for _, want := range wantContain {
		if !slices.Contains(args, want) {
			t.Errorf("args missing %q in: %s", want, joined)
		}
	}
	for _, exclude := range wantExclude {
		if slices.Contains(args, exclude) {
			t.Errorf("args should not contain %q in: %s", exclude, joined)
		}
	}
}
