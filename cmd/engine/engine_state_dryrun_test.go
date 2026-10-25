package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// savedEngineImageFixture is the valid engine image state a dry-run preview
// must preserve (#733): its image was built by a previous real build.
var savedEngineImageFixture = &state.EngineImageState{
	ImageTag: "ludus-engine:real",
	BuiltAt:  "2026-01-01T00:00:00Z",
}

// containerEngineCfg returns a config whose container engine build succeeds
// in dry-run (the runner echoes docker instead of executing it) and resets the
// engine flags for the duration of the test.
func containerEngineCfg(t *testing.T, dryRun bool) *config.Config {
	t.Helper()
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: engineRoot, Version: "5.7.3", Backend: "docker"},
		Game:   config.GameConfig{ProjectName: "TestGame"},
	}
	origNoCache, origSkip := noCache, skipEngine
	noCache, skipEngine = true, true
	t.Cleanup(func() { noCache, skipEngine = origNoCache, origSkip })
	globals.SetGlobals(t, cfg, globals.WithDryRun(dryRun))
	t.Chdir(t.TempDir())
	// skip-engine needs pre-built Linux binaries present.
	binDir := filepath.Join(engineRoot, "Engine", "Binaries", "Linux")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "UnrealEditor"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// seedEngineImageState writes the saved engine image fixture into the
// current working directory's state file, after the test's chdir.
func seedEngineImageState(t *testing.T) {
	t.Helper()
	origProfile := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(origProfile) })
	if err := state.UpdateEngineImage(savedEngineImageFixture); err != nil {
		t.Fatalf("seeding saved engine image state: %v", err)
	}
}

// assertEngineImagePreserved verifies the saved engine image survived a
// dry-run build (#733).
func assertEngineImagePreserved(t *testing.T) {
	t.Helper()
	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	if s.EngineImage == nil {
		t.Fatal("saved engine image state was removed by a dry-run build")
	}
	if s.EngineImage.ImageTag != savedEngineImageFixture.ImageTag {
		t.Errorf("saved engine image overwritten by a dry-run build: got %q, want %q",
			s.EngineImage.ImageTag, savedEngineImageFixture.ImageTag)
	}
}

// TestRunContainerBuildDryRunPreservesEngineImage pins #733: a dry-run
// container engine build reports success without overwriting the saved
// engine image with a tag that was never built.
func TestRunContainerBuildDryRunPreservesEngineImage(t *testing.T) {
	cfg := containerEngineCfg(t, true)
	seedEngineImageState(t)

	output := captureStdout(func() {
		if err := runContainerBuild(engineCmd(t), "docker", cfg); err != nil {
			t.Fatalf("runContainerBuild(dry-run) error = %v", err)
		}
	})
	if !strings.Contains(output, "Engine image built") {
		t.Errorf("runContainerBuild(dry-run) output = %q, want the success message", output)
	}

	assertEngineImagePreserved(t)
}

// TestRunContainerBuildRealBuildRecordsEngineImage pins the real counterpart
// of #733: a non-dry-run container engine build records its image tag in
// state (docker is stubbed so no real build runs).
func TestRunContainerBuildRealBuildRecordsEngineImage(t *testing.T) {
	cfg := containerEngineCfg(t, false)
	seedEngineImageState(t)
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{})

	output := captureStdout(func() {
		if err := runContainerBuild(engineCmd(t), "docker", cfg); err != nil {
			t.Fatalf("runContainerBuild() error = %v", err)
		}
	})
	if !strings.Contains(output, "Engine image built") {
		t.Errorf("runContainerBuild() output = %q, want the success message", output)
	}

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	if s.EngineImage == nil {
		t.Fatal("real engine image build did not record state")
	}
	if s.EngineImage.ImageTag == savedEngineImageFixture.ImageTag {
		t.Errorf("recorded image tag = %q, want the built tag", s.EngineImage.ImageTag)
	}
}

// TestSaveWSL2EngineStateSkippedUnderDryRun pins #733 for the shared WSL
// engine state helper: a dry-run preview must not overwrite the saved WSL2
// engine paths.
func TestSaveWSL2EngineStateSkippedUnderDryRun(t *testing.T) {
	t.Chdir(t.TempDir())
	origProfile := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(origProfile) })
	saved := &state.WSL2EngineState{EnginePath: "/mnt/c/real", DDCPath: "/mnt/c/real/ddc", BuiltAt: "2026-01-01T00:00:00Z"}
	if err := state.UpdateWSL2Engine(saved); err != nil {
		t.Fatalf("seeding saved WSL2 engine state: %v", err)
	}
	origNative := wslNative
	wslNative = true
	t.Cleanup(func() { wslNative = origNative })
	globals.SetGlobals(t, &config.Config{}, globals.WithDryRun(true))

	saveWSL2EngineState("/planned/wsl/engine", "/planned/wsl/ddc")

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	if s.WSL2Engine == nil {
		t.Fatal("saved WSL2 engine state was removed by a dry-run build")
	}
	if s.WSL2Engine.EnginePath != saved.EnginePath {
		t.Errorf("saved WSL2 engine overwritten by a dry-run build: got %q, want %q",
			s.WSL2Engine.EnginePath, saved.EnginePath)
	}
}
