package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/jpvelasco/ludus/internal/wsl"
	"github.com/spf13/cobra"
)

// quietRunner returns a real (non-dry-run) runner with output silenced so the
// stubbed wsl.exe on PATH is actually executed during path-resolution tests.
func quietRunner() *runner.Runner {
	r := runner.NewRunner(false, false)
	r.Stdout = &strings.Builder{}
	r.Stderr = &strings.Builder{}
	return r
}

func TestResolveWSL2EnginePathsNativeSync(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "  250G"})

	wslNative = true
	t.Cleanup(func() { wslNative = false })

	w := &wsl.WSL2{Distro: "Ubuntu", Runner: quietRunner()}
	c := &cobra.Command{}
	c.SetContext(context.Background())

	enginePath, ddcPath, err := resolveWSL2EnginePaths(c, quietRunner(), w, `C:\ue5`, "5.7")
	if err != nil {
		t.Fatalf("resolveWSL2EnginePaths(native) error = %v", err)
	}
	if enginePath != "$HOME/ludus/engine/5.7" {
		t.Errorf("enginePath = %q, want $HOME/ludus/engine/5.7", enginePath)
	}
	if ddcPath != "$HOME/ludus/ddc" {
		t.Errorf("ddcPath = %q, want $HOME/ludus/ddc", ddcPath)
	}
}

func TestResolveWSL2EnginePathsNativeInsufficientDisk(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "  10G"})

	wslNative = true
	t.Cleanup(func() { wslNative = false })

	w := &wsl.WSL2{Distro: "Ubuntu", Runner: quietRunner()}
	c := &cobra.Command{}
	c.SetContext(context.Background())

	_, _, err := resolveWSL2EnginePaths(c, quietRunner(), w, `C:\ue5`, "5.7")
	if err == nil || !strings.Contains(err.Error(), "insufficient disk space") {
		t.Errorf("resolveWSL2EnginePaths() error = %v, want 'insufficient disk space'", err)
	}
}

func TestResolveWSL2EnginePathsNativeDiskCheckError(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})

	wslNative = true
	t.Cleanup(func() { wslNative = false })

	w := &wsl.WSL2{Distro: "Ubuntu", Runner: quietRunner()}
	c := &cobra.Command{}
	c.SetContext(context.Background())

	_, _, err := resolveWSL2EnginePaths(c, quietRunner(), w, `C:\ue5`, "5.7")
	if err == nil || !strings.Contains(err.Error(), "checking disk space") {
		t.Errorf("resolveWSL2EnginePaths() error = %v, want 'checking disk space'", err)
	}
}

// TestRunWSL2BuildSuccess drives the full WSL2 engine build under a stubbed
// wsl.exe. The stub's stdout doubles as both the `wsl --list --verbose` distro
// list (for wsl.New) and a non-empty `which` result (for CheckDeps); exit 0
// makes every build step succeed. State is written to a temp dir.
func TestRunWSL2BuildSuccess(t *testing.T) {
	t.Chdir(t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })

	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{
		Stdout: "* Ubuntu          Running         2",
	})

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: `C:\ue5`,
			Version:    "5.7",
			MaxJobs:    1,
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(false), globals.WithNoLogs(true))

	wslNative = false
	t.Cleanup(func() { wslNative = false })

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	if err := runWSL2Build(cmd); err != nil {
		t.Fatalf("runWSL2Build() error = %v", err)
	}
}

// TestSaveWSL2EngineState_WarnOnWriteFailure covers the state-write failure
// branch of saveWSL2EngineState (engine.go:443-445): the function must not
// return an error, and it must print a warning instead of panicking.
func TestSaveWSL2EngineState_WarnOnWriteFailure(t *testing.T) {
	previous := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(previous) })
	testsupport.BlockStateWrite(t)

	wslNative = false
	t.Cleanup(func() { wslNative = false })

	output := captureStdout(func() {
		saveWSL2EngineState("/home/ue/Engine", "/home/ue/.ludus/ddc")
	})

	if !strings.Contains(output, "Warning: failed to write state") {
		t.Errorf("output = %q, want a 'Warning: failed to write state' message", output)
	}
}
