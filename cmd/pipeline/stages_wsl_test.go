package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/jpvelasco/ludus/internal/wsl"
)

func TestResolveWSL2GameDDCPathFromState(t *testing.T) {
	w := &wsl.WSL2{Distro: "Ubuntu-24.04"}

	engineState := &state.WSL2EngineState{
		DDCPath: "/home/user/ludus/ddc",
	}

	got := resolveWSL2GameDDCPath(engineState, "local", "C:/ludus/ddc", w)
	if got != "/home/user/ludus/ddc" {
		t.Errorf("resolveWSL2GameDDCPath() = %q, want %q", got, "/home/user/ludus/ddc")
	}
}

func TestResolveWSL2GameDDCPathFallbackVirtiofs(t *testing.T) {
	w := &wsl.WSL2{Distro: "Ubuntu-24.04"}

	engineState := &state.WSL2EngineState{
		DDCPath: "",
	}

	got := resolveWSL2GameDDCPath(engineState, "local", `C:\ludus\ddc`, w)
	if got == "" {
		t.Errorf("resolveWSL2GameDDCPath() returned empty string, want path")
	}
}

func TestResolveWSL2GameDDCPathModeNone(t *testing.T) {
	w := &wsl.WSL2{Distro: "Ubuntu-24.04"}

	engineState := &state.WSL2EngineState{
		DDCPath: "",
	}

	got := resolveWSL2GameDDCPath(engineState, "none", `C:\ludus\ddc`, w)
	if got != "" {
		t.Errorf("resolveWSL2GameDDCPath() = %q, want empty for mode=none", got)
	}
}

func TestWSL2Fallback(t *testing.T) {
	err := wsl2Fallback(nil)
	if err == nil {
		t.Fatal("wsl2Fallback() expected error, got nil")
	}
}

// TestResolveWSL2EnginePathsNativeSync covers the wslNative=true branch of
// resolveWSL2EnginePaths (stages_wsl.go:53-63): the stubbed wsl.exe reports
// enough disk space, so SyncEngine returns the native ext4 paths. The stub
// output is a single line because the Windows .bat stub echoes each line as a
// separate command.
func TestResolveWSL2EnginePathsNativeSync(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "250G"})

	p := newTestPipelineCtx(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: `C:\ue5`},
	}, &testContextOpts{engineVersion: "5.7"})
	p.wslNative = true

	enginePath, ddcPath, err := p.resolveWSL2EnginePaths(context.Background(), &wsl.WSL2{Distro: "Ubuntu"})
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

// TestResolveWSL2EnginePathsNativeInsufficientDisk covers the SyncEngine error
// branch of resolveWSL2EnginePaths (stages_wsl.go:59-61): the stub reports too
// little disk space, so the sync error propagates.
func TestResolveWSL2EnginePathsNativeInsufficientDisk(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "10G"})

	p := newTestPipelineCtx(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: `C:\ue5`},
	}, &testContextOpts{engineVersion: "5.7"})
	p.wslNative = true

	_, _, err := p.resolveWSL2EnginePaths(context.Background(), &wsl.WSL2{Distro: "Ubuntu"})
	if err == nil || !strings.Contains(err.Error(), "insufficient disk space") {
		t.Fatalf("resolveWSL2EnginePaths() error = %v, want 'insufficient disk space'", err)
	}
}

// TestBuildEngineWSL2WSL2Unavailable covers the wsl.New failure branch of
// buildEngineWSL2 (stages_wsl.go:25-26): the stub wsl.exe exits non-zero, so
// the error is wrapped with the Podman fallback hint.
func TestBuildEngineWSL2WSL2Unavailable(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})

	p := newTestPipelineCtx(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: `C:\ue5`, MaxJobs: 4},
	}, nil)

	_, err := p.buildEngineWSL2(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Podman instead") {
		t.Fatalf("buildEngineWSL2() error = %v, want Podman fallback hint", err)
	}
}

// TestBuildEngineWSL2Success drives the full WSL2 engine build success path
// (stages_wsl.go:28-46): detection, the virtiofs path resolution, all build
// steps, and state persistence succeed against the stubbed wsl.exe.
func TestBuildEngineWSL2Success(t *testing.T) {
	t.Chdir(t.TempDir())
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "* Ubuntu Running 2"})

	p := newTestPipelineCtx(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: `C:\ue5`, MaxJobs: 4, Version: "5.7.3"},
	}, nil)

	result, err := p.buildEngineWSL2(context.Background())
	if err != nil {
		t.Fatalf("buildEngineWSL2() error = %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("buildEngineWSL2() result = %+v, want success", result)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if st.WSL2Engine == nil || st.WSL2Engine.EnginePath != "/mnt/c/ue5" {
		t.Errorf("WSL2Engine state = %+v, want virtiofs engine path /mnt/c/ue5", st.WSL2Engine)
	}
}

// TestSaveWSL2EngineStateNativeSyncTime covers the wslNative syncTime branch of
// saveWSL2EngineState (stages_wsl.go:73-76) and its write-error fallback: the
// state file is a directory, so UpdateWSL2Engine fails and only a warning is
// printed, with no panic.
func TestSaveWSL2EngineStateNativeSyncTime(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join(dir, ".ludus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".ludus", "state.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := newTestPipelineCtx(t, &config.Config{}, nil)
	p.wslNative = true

	p.saveWSL2EngineState("/wsl/engine", "/wsl/ddc")
}

// TestBuildGameWSL2NoEngineState covers the missing-engine-state branch of
// buildGameWSL2 (stages_wsl.go:96-98): with an empty state, the build fails
// before any WSL2 interaction.
func TestBuildGameWSL2NoEngineState(t *testing.T) {
	t.Chdir(t.TempDir())

	p := newTestPipelineCtx(t, &config.Config{
		Game: config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
	}, nil)

	_, err := p.buildGameWSL2(context.Background(), "Lyra")
	if err == nil || !strings.Contains(err.Error(), "no WSL2 engine build found") {
		t.Fatalf("buildGameWSL2() error = %v, want 'no WSL2 engine build found'", err)
	}
}

// TestBuildGameWSL2StateLoadError covers the state.Load failure branch of
// buildGameWSL2 (stages_wsl.go:93-95): a corrupt state file surfaces as an
// error before any WSL2 interaction.
func TestBuildGameWSL2StateLoadError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join(dir, ".ludus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ludus", "state.json"), []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newTestPipelineCtx(t, &config.Config{}, nil)

	_, err := p.buildGameWSL2(context.Background(), "Lyra")
	if err == nil || !strings.Contains(err.Error(), "loading state") {
		t.Fatalf("buildGameWSL2() error = %v, want 'loading state'", err)
	}
}

// TestBuildGameWSL2PassesConfiguredOutputDir pins the contract that the WSL2
// game stage archives into the pipeline's serverBuildDir — the directory the
// container-build hash and deploy stage read afterwards. The recorded command
// line must carry -archivedirectory pointing at the WSL-mapped build dir,
// never an empty value (which would send UAT output to its default location).
func TestBuildGameWSL2PassesConfiguredOutputDir(t *testing.T) {
	t.Chdir(t.TempDir())
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{Stdout: "* Ubuntu Running 2"})

	if err := state.UpdateWSL2Engine(&state.WSL2EngineState{EnginePath: "/mnt/c/ue5"}); err != nil {
		t.Fatalf("seeding WSL2Engine state: %v", err)
	}

	p := newTestPipelineCtx(t, &config.Config{
		Game: config.GameConfig{ProjectName: "Lyra", ProjectPath: `C:\game\Lyra.uproject`},
	}, &testContextOpts{withRecordingR: true})
	rr, readCommands := testsupport.RecordingRunner()
	p.r = rr
	p.serverBuildDir = `C:\game\PackagedServer\LinuxServer`

	result, err := p.buildGameWSL2(context.Background(), "Lyra")
	if err != nil {
		t.Fatalf("buildGameWSL2() error = %v", err)
	}
	if result == nil || result.OutputDir != "/mnt/c/game/PackagedServer/LinuxServer" {
		t.Errorf("buildGameWSL2() OutputDir = %+v, want /mnt/c/game/PackagedServer/LinuxServer", result)
	}

	joined := strings.Join(readCommands(), "\n")
	const wantArg = "-archivedirectory='/mnt/c/game/PackagedServer/LinuxServer'"
	if !strings.Contains(joined, wantArg) {
		t.Errorf("recorded commands missing %s; got:\n%s", wantArg, joined)
	}
}
