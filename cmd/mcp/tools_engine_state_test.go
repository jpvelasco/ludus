package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// savedEngineImage is the valid engine image state a dry-run preview must
// preserve (#733): its image was built by a previous real build.
var savedEngineImage = &state.EngineImageState{
	ImageTag: "ludus-engine:real",
	Version:  "5.7.3",
	BuiltAt:  "2026-01-01T00:00:00Z",
}

// savedWSL2Engine is the valid WSL2 engine state a dry-run preview must
// preserve (#733).
var savedWSL2Engine = &state.WSL2EngineState{
	EnginePath: "/mnt/e/engine",
	DDCPath:    "/mnt/e/.ludus/ddc",
	BuiltAt:    "2026-01-01T00:00:00Z",
}

// seedSavedEngineState writes the engine image fixture into a fresh working
// directory's state file.
func seedSavedEngineState(t *testing.T) {
	t.Helper()
	freshStateScope(t)
	if err := state.UpdateEngineImage(savedEngineImage); err != nil {
		t.Fatalf("seeding saved engine image state: %v", err)
	}
}

// assertEngineImagePreserved checks that the saved engine image survived a
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
	if s.EngineImage.ImageTag != savedEngineImage.ImageTag {
		t.Errorf("saved engine image overwritten by a dry-run build: got %q, want %q",
			s.EngineImage.ImageTag, savedEngineImage.ImageTag)
	}
}

// dryRunContainerEngineCfg returns a config whose container engine build
// succeeds in dry-run (the runner echoes docker instead of executing it).
func dryRunContainerEngineCfg(t *testing.T) *config.Config {
	t.Helper()
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: engineRoot, Version: "5.7.3", Backend: "docker"},
		Game:   config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
	}
	globals.SetGlobals(t, cfg, globals.WithNoLogs(true))
	return cfg
}

// TestHandleContainerEngineBuildDryRunPreservesEngineImage pins #733 for the
// sync container engine build: a dry-run preview reports success without
// building the image, and the saved engine image must stay untouched.
func TestHandleContainerEngineBuildDryRunPreservesEngineImage(t *testing.T) {
	seedSavedEngineState(t)
	cfg := dryRunContainerEngineCfg(t)

	result, _, err := handleContainerEngineBuild(context.Background(), cfg,
		engineBuildInput{NoCache: true, DryRun: true}, "docker")
	if err != nil {
		t.Fatalf("handleContainerEngineBuild() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handleContainerEngineBuild() dry-run failed: %s", toolResultText(t, result))
	}

	assertEngineImagePreserved(t)
}

// saveEngineImageStateCase drives one scenario of the engine image state
// helper: the dry-run guard skips the write, a real save records the built
// image, and a write failure surfaces for the caller to warn about.
type saveEngineImageStateCase struct {
	name       string
	dryRun     bool
	blockState bool
	wantErr    bool
	wantSaved  bool
}

// TestSaveEngineImageStateHelper covers the shared engine image state helper
// (#733), mirroring the client-state helper's coverage.
func TestSaveEngineImageStateHelper(t *testing.T) {
	tests := []saveEngineImageStateCase{
		{name: "dry-run preview skips the write", dryRun: true, wantSaved: false},
		{name: "real build saves the image", dryRun: false, wantSaved: true},
		{name: "write failure surfaces", dryRun: false, blockState: true, wantErr: true, wantSaved: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runSaveEngineImageStateCase(t, tt)
		})
	}
}

func runSaveEngineImageStateCase(t *testing.T, tt saveEngineImageStateCase) {
	t.Helper()
	freshStateScope(t)
	if tt.blockState {
		if err := os.WriteFile(".ludus", []byte("not a directory"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err := saveEngineImageState(tt.dryRun, "ludus-engine:built", "5.7.3")

	if tt.wantErr != (err != nil) {
		t.Fatalf("saveEngineImageState() error = %v, want error = %v", err, tt.wantErr)
	}
	if tt.wantErr && !strings.Contains(persistFailedWarn(err), ".ludus state could not be updated") {
		t.Errorf("persistence error should produce the warning, got: %v", err)
	}
	if tt.blockState {
		return // the blocked state file cannot be loaded back
	}
	assertSavedEngineImage(t, tt.wantSaved)
}

// assertSavedEngineImage checks whether the engine image block matches wantSaved.
func assertSavedEngineImage(t *testing.T, wantSaved bool) {
	t.Helper()
	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	if gotSaved := s.EngineImage != nil; gotSaved != wantSaved {
		t.Fatalf("engine image saved = %v, want %v (state: %+v)", gotSaved, wantSaved, s.EngineImage)
	}
	if wantSaved && s.EngineImage.ImageTag != "ludus-engine:built" {
		t.Errorf("saved image tag = %q, want ludus-engine:built", s.EngineImage.ImageTag)
	}
}

// freshStateScope chdirs to a temp dir and resets the active state profile so
// each test reads and writes its own .ludus/state.json.
func freshStateScope(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	origProfile := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(origProfile) })
}

// TestSaveWSL2EngineResultDryRunPreservesState pins #733 for the WSL2 engine
// state helper: a dry-run preview must not overwrite the saved engine with
// paths that were never built.
func TestSaveWSL2EngineResultDryRunPreservesState(t *testing.T) {
	freshStateScope(t)
	if err := state.UpdateWSL2Engine(savedWSL2Engine); err != nil {
		t.Fatalf("seeding WSL2 engine state: %v", err)
	}

	if err := saveWSL2EngineResult("/preview/engine", "", "hash", false, true); err != nil {
		t.Fatalf("saveWSL2EngineResult() dry-run error = %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.WSL2Engine.EnginePath != savedWSL2Engine.EnginePath {
		t.Errorf("saved WSL2 engine overwritten by a dry-run build: got %q, want %q",
			s.WSL2Engine.EnginePath, savedWSL2Engine.EnginePath)
	}
}

// TestSaveWSL2EngineResultSavesRealBuild covers the non-dry-run side: a real
// build records its engine and DDC paths in state.
func TestSaveWSL2EngineResultSavesRealBuild(t *testing.T) {
	freshStateScope(t)
	if err := saveWSL2EngineResult("/mnt/e/engine", "/mnt/e/ddc", "hash", false, false); err != nil {
		t.Fatalf("saveWSL2EngineResult() error = %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.WSL2Engine == nil || s.WSL2Engine.EnginePath != "/mnt/e/engine" {
		t.Errorf("real build must save the WSL2 engine, got %+v", s.WSL2Engine)
	}
}

// TestFakeEngineTreeProvidesSetupScript verifies the engine tree fixture the
// dry-run container config relies on: the container builder resolves the
// engine source only when the setup script exists.
func TestFakeEngineTreeProvidesSetupScript(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	if _, err := os.Stat(filepath.Join(engineRoot, engineSetupScript())); err != nil {
		t.Errorf("FakeEngineTree missing %s: %v", engineSetupScript(), err)
	}
}

// TestApplyEngineBuildResultNil covers the nil-tolerance of the engine result
// assembly helper: a builder returning no result must not touch the fields.
func TestApplyEngineBuildResultNil(t *testing.T) {
	var result engineResult
	result.EnginePath = "/engine"
	applyEngineBuildResult(nil, &result)
	if result.Success || result.DurationSeconds != 0 {
		t.Errorf("nil build result must not populate fields, got %+v", result)
	}
}
