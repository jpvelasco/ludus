package game

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/spf13/cobra"
)

// TestRunWSL2GameBuildSuccess drives the full WSL2 game build under a stubbed
// wsl.exe. The stub's stdout doubles as both the `wsl --list --verbose` distro
// list (for wsl.New) and a non-empty `ldconfig` result (for CheckRuntimeDeps);
// the dry-run runner makes every build step a no-op. State is written to a
// temp dir with a prior WSL2 engine build.
func TestRunWSL2GameBuildSuccess(t *testing.T) {
	t.Chdir(t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })

	projectPath := testsupport.FakeProject(t, "TestGame")

	if err := state.UpdateWSL2Engine(&state.WSL2EngineState{
		EnginePath: "/home/user/ludus/engine/5.7",
		DDCPath:    "/home/user/ludus/ddc",
		IsNative:   true,
	}); err != nil {
		t.Fatalf("UpdateWSL2Engine() error = %v", err)
	}

	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{
		Stdout: "* Ubuntu          Running         2",
	})

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: `C:\ue5`,
			Version:    "5.7",
			MaxJobs:    1,
		},
		Game: config.GameConfig{
			ProjectName:  "TestGame",
			ProjectPath:  projectPath,
			Platform:     "Linux",
			ServerTarget: "TestGameServer",
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithDDCMode("none"))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runWSL2GameBuild(cmd, cfg)
	if err != nil {
		t.Fatalf("runWSL2GameBuild() error = %v", err)
	}
}

// TestRunWSL2GameBuildRealRunner drives the WSL2 game build with a real
// (non-dry-run) runner: the wsl.exe stub answers every probe (distro list,
// ldconfig) with exit 0, so the build proceeds end to end without a live WSL2.
func TestRunWSL2GameBuildRealRunner(t *testing.T) {
	t.Chdir(t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })

	projectPath := testsupport.FakeProject(t, "TestGame")

	if err := state.UpdateWSL2Engine(&state.WSL2EngineState{
		EnginePath: "/home/user/ludus/engine/5.7",
		DDCPath:    "/home/user/ludus/ddc",
		IsNative:   true,
	}); err != nil {
		t.Fatalf("UpdateWSL2Engine() error = %v", err)
	}

	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{
		Stdout: "* Ubuntu          Running         2",
	})

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: `C:\ue5`,
			Version:    "5.7",
			MaxJobs:    1,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: projectPath,
			Platform:    "Linux",
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(false), globals.WithNoLogs(true), globals.WithDDCMode("none"))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runWSL2GameBuild(cmd, cfg)
	if err != nil {
		t.Fatalf("runWSL2GameBuild() error = %v", err)
	}
}
