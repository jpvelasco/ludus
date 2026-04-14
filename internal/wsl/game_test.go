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

func TestBuildGameScript(t *testing.T) {
	tests := []struct {
		name           string
		opts           GameOptions
		projectPath    string
		outputDir      string
		wantContains   []string
		wantNotContain []string
	}{
		{
			// UE-LocalDataCachePath has a hyphen — not a valid shell identifier.
			// Must use env "KEY=VALUE", not export or any other shell assignment.
			name: "DDC local: uses env not export",
			opts: GameOptions{
				EnginePath: "/opt/ue/5.7",
				DDCMode:    ddc.ModeLocal,
				DDCPath:    "/home/user/ddc",
			},
			projectPath: "/mnt/f/game/MyGame.uproject",
			outputDir:   "/mnt/f/out",
			wantContains: []string{
				`env 'UE-LocalDataCachePath=`,
			},
			wantNotContain: []string{
				"export UE-LocalDataCachePath",
				"export UE-",
			},
		},
		{
			// buildGameScript receives pre-resolved paths (ExpandHomePaths runs in
			// BuildGame before this function is called). shellQuote single-quotes the
			// resolved path so spaces and special chars are preserved verbatim.
			name: "native path: pre-resolved absolute paths are single-quoted",
			opts: GameOptions{
				EnginePath: "/home/user/ludus/engine/5.7",
				DDCMode:    ddc.ModeLocal,
				DDCPath:    "/home/user/ludus/ddc",
			},
			projectPath: "/mnt/f/game/MyGame.uproject",
			outputDir:   "/mnt/f/out",
			wantContains: []string{
				`cd '/home/user/ludus/engine/5.7'`,
				`env 'UE-LocalDataCachePath=/home/user/ludus/ddc'`,
			},
		},
		{
			// DDC path with spaces must remain intact inside the double-quoted
			// env "KEY=VALUE" argument — spaces in the value are not word-split.
			name: "DDC local: path with spaces preserved in env arg",
			opts: GameOptions{
				EnginePath: "/opt/ue/5.7",
				DDCMode:    ddc.ModeLocal,
				DDCPath:    "/home/user/my ddc",
			},
			projectPath: "/mnt/f/game/MyGame.uproject",
			outputDir:   "/mnt/f/out",
			wantContains: []string{
				`env 'UE-LocalDataCachePath=/home/user/my ddc'`,
			},
		},
		{
			name: "DDC none: no env prefix",
			opts: GameOptions{
				EnginePath: "/opt/ue/5.7",
				DDCMode:    ddc.ModeNone,
			},
			projectPath: "/mnt/f/game/MyGame.uproject",
			outputDir:   "/mnt/f/out",
			wantNotContain: []string{
				"UE-LocalDataCachePath",
				"env ",
			},
		},
		{
			name: "DDC local with empty path: no env prefix",
			opts: GameOptions{
				EnginePath: "/opt/ue/5.7",
				DDCMode:    ddc.ModeLocal,
				DDCPath:    "",
			},
			projectPath: "/mnt/f/game/MyGame.uproject",
			outputDir:   "/mnt/f/out",
			wantNotContain: []string{
				"UE-LocalDataCachePath",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := buildGameScript(tt.opts, tt.projectPath, tt.outputDir)
			for _, want := range tt.wantContains {
				if !strings.Contains(script, want) {
					t.Errorf("script missing %q\ngot: %s", want, script)
				}
			}
			for _, notWant := range tt.wantNotContain {
				if strings.Contains(script, notWant) {
					t.Errorf("script should not contain %q\ngot: %s", notWant, script)
				}
			}
		})
	}
}

func TestBuildRunUATArgs(t *testing.T) {
	tests := []struct {
		name        string
		opts        GameOptions
		projectPath string
		outputDir   string
		wantContain []string
		wantExclude []string
	}{
		{
			"basic server build",
			GameOptions{Platform: "Linux", ServerConfig: "Development"},
			"/mnt/f/game/MyGame.uproject",
			"/mnt/f/game/PackagedServer",
			[]string{
				"BuildCookRun",
				"-project='/mnt/f/game/MyGame.uproject'",
				"-platform=Linux",
				"-server",
				"-noclient",
				"-cook",
				"-serverconfig=Development",
			},
			nil,
		},
		{
			"with server target and map",
			GameOptions{
				ServerTarget: "LyraServer",
				ServerMap:    "/Game/Maps/Expanse",
			},
			"/mnt/f/game/Lyra.uproject",
			"/mnt/f/out",
			[]string{
				"-target=LyraServer",
				"-map=/Game/Maps/Expanse",
			},
			nil,
		},
		{
			"skip cook",
			GameOptions{SkipCook: true},
			"/mnt/f/game/My.uproject",
			"/mnt/f/out",
			[]string{"-skipcook"},
			[]string{"-cook"},
		},
		{
			"with max jobs",
			GameOptions{MaxJobs: 8},
			"/mnt/f/game/My.uproject",
			"/mnt/f/out",
			[]string{"-MaxParallelActions=8"},
			nil,
		},
		{
			"defaults to Linux and Development",
			GameOptions{},
			"/proj",
			"/out",
			[]string{"-platform=Linux", "-serverconfig=Development"},
			nil,
		},
		{
			// Windows paths often contain spaces (e.g. "Source Code/...").
			// The project and archive args must be single-quoted so bash does not
			// word-split on the space.
			"path with spaces: project and archive are single-quoted",
			GameOptions{Platform: "Linux"},
			"/mnt/f/Source Code/MyGame.uproject",
			"/mnt/f/Source Code/Packaged",
			[]string{
				"-project='/mnt/f/Source Code/MyGame.uproject'",
				"-archivedirectory='/mnt/f/Source Code/Packaged'",
			},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildRunUATArgs(tt.opts, tt.projectPath, tt.outputDir)
			joined := strings.Join(args, " ")

			for _, want := range tt.wantContain {
				if !slices.Contains(args, want) {
					t.Errorf("args missing %q in: %s", want, joined)
				}
			}

			for _, exclude := range tt.wantExclude {
				if slices.Contains(args, exclude) {
					t.Errorf("args should not contain %q in: %s", exclude, joined)
				}
			}
		})
	}
}
