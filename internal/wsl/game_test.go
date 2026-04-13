package wsl

import (
	"slices"
	"strings"
	"testing"
)

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
				"-project=/mnt/f/game/MyGame.uproject",
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
