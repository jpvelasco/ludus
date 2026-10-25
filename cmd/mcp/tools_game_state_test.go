package mcp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// savedClient is the valid client state a dry-run preview must preserve:
// its binary was actually built by a previous real build.
var savedClient = &state.ClientState{
	BinaryPath: "/real/builds/Lyra.sh",
	OutputDir:  "/real/builds",
	Platform:   "Linux",
	BuiltAt:    "2026-01-01T00:00:00Z",
}

// seedSavedClient writes the valid saved client state into a fresh working
// directory and returns the loaded state for comparisons.
func seedSavedClient(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	origProfile := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(origProfile) })
	if err := state.UpdateClient(savedClient); err != nil {
		t.Fatalf("seeding saved client state: %v", err)
	}
}

// assertClientPreserved checks that the saved client state survived a
// dry-run build attempt (#697).
func assertClientPreserved(t *testing.T) {
	t.Helper()
	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	if s.Client == nil {
		t.Fatal("saved client state was removed by a dry-run build")
	}
	fields := map[string]struct {
		got  string
		want string
	}{
		"binary":   {got: s.Client.BinaryPath, want: savedClient.BinaryPath},
		"output":   {got: s.Client.OutputDir, want: savedClient.OutputDir},
		"platform": {got: s.Client.Platform, want: savedClient.Platform},
		"builtAt":  {got: s.Client.BuiltAt, want: savedClient.BuiltAt},
	}
	for name, field := range fields {
		if field.got != field.want {
			t.Errorf("saved client %s overwritten by a dry-run build: got %q, want %q", name, field.got, field.want)
		}
	}
}

// nativeClientCfg returns a config whose native dry-run client build succeeds
// (RunUAT resolves and the runner echoes instead of executing).
func nativeClientCfg(t *testing.T) *config.Config {
	t.Helper()
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "Lyra")
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: engineRoot, Version: "5.7.3", Backend: "native"},
		Game:   config.GameConfig{ProjectName: "Lyra", ProjectPath: projectPath},
	}
	globals.SetGlobals(t, cfg, globals.WithDDCMode("none"))
	return cfg
}

// TestHandleGameClientDryRunPreservesSavedClient pins #697 for the sync
// native client build: a dry-run preview reports the planned binary, and the
// saved client state must stay untouched.
func TestHandleGameClientDryRunPreservesSavedClient(t *testing.T) {
	seedSavedClient(t)
	nativeClientCfg(t)

	result, _, err := handleGameClient(context.Background(), nil, gameClientInput{NoCache: true, DryRun: true})
	if err != nil {
		t.Fatalf("handleGameClient() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handleGameClient() dry-run failed: %s", toolResultText(t, result))
	}

	assertClientPreserved(t)
}

// TestHandleContainerGameClientDryRunPreservesSavedClient pins #697 for the
// sync container client build.
func TestHandleContainerGameClientDryRunPreservesSavedClient(t *testing.T) {
	seedSavedClient(t)
	cfg := &config.Config{
		Engine: config.EngineConfig{DockerImage: "engine:5.7", Backend: "docker"},
		Game:   config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
	}
	globals.SetGlobals(t, cfg, globals.WithDDCMode("none"))
	// The seeded working directory already carries state; the project file is
	// written next to it so the container build resolves it.
	if err := os.WriteFile("Lyra.uproject", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, _, err := handleContainerGameClient(context.Background(), cfg, gameClientInput{NoCache: true, DryRun: true}, "Linux", "docker")
	if err != nil {
		t.Fatalf("handleContainerGameClient() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handleContainerGameClient() dry-run failed: %s", toolResultText(t, result))
	}

	assertClientPreserved(t)
}

// TestGameClientStartDryRunPreservesSavedClient pins #697 for the async
// client build: the goroutine's success path must not persist the planned
// client either.
func TestGameClientStartDryRunPreservesSavedClient(t *testing.T) {
	seedSavedClient(t)
	nativeClientCfg(t)
	withBuildManager(t)

	result, _, err := handleGameClientStart(context.Background(), nil, gameClientStartInput{
		Backend: "native", NoCache: true, DryRun: true,
	})
	if err != nil {
		t.Fatalf("handleGameClientStart() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handleGameClientStart() failed: %s", toolResultText(t, result))
	}

	var started buildStartResult
	if err := json.Unmarshal([]byte(toolResultText(t, result)), &started); err != nil {
		t.Fatalf("decoding build start result: %v", err)
	}
	waitForTerminal(t, builds, started.BuildID)

	assertClientPreserved(t)
}

// saveClientStateCase drives one scenario of the shared MCP client-state
// helper: the dry-run guard skips the write, a real save records the built
// client, and a write failure surfaces for the caller to warn about.
type saveClientStateCase struct {
	name       string
	dryRun     bool
	blockState bool
	wantErr    bool
	wantSaved  bool
}

// TestSaveClientStateHelper covers the shared MCP client-state helper (#697).
func TestSaveClientStateHelper(t *testing.T) {
	tests := []saveClientStateCase{
		{name: "dry-run preview skips the write", dryRun: true, wantSaved: false},
		{name: "real build saves the client", dryRun: false, wantSaved: true},
		{name: "write failure surfaces", dryRun: false, blockState: true, wantErr: true, wantSaved: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runSaveClientStateCase(t, tt)
		})
	}
}

func runSaveClientStateCase(t *testing.T, tt saveClientStateCase) {
	t.Helper()
	t.Chdir(t.TempDir())
	origProfile := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(origProfile) })
	if tt.blockState {
		if err := os.WriteFile(".ludus", []byte("not a directory"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result := gameBuildResult{Success: true, Binary: "/built/client", OutputDir: "/built"}
	err := saveClientState(tt.dryRun, "Linux", &result)

	if tt.wantErr != (err != nil) {
		t.Fatalf("saveClientState() error = %v, want error = %v", err, tt.wantErr)
	}
	if tt.wantErr && !strings.Contains(persistFailedWarn(err), ".ludus state could not be updated") {
		t.Errorf("persistence error should produce the warning, got: %v", err)
	}
	if tt.blockState {
		return // the blocked state file cannot be loaded back
	}
	assertSavedClientState(t, tt.wantSaved)
}

// assertSavedClientState checks whether the client block matches wantSaved.
func assertSavedClientState(t *testing.T, wantSaved bool) {
	t.Helper()
	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	if gotSaved := s.Client != nil; gotSaved != wantSaved {
		t.Fatalf("client saved = %v, want %v (state: %+v)", gotSaved, wantSaved, s.Client)
	}
	if wantSaved && s.Client.BinaryPath != "/built/client" {
		t.Errorf("saved client binary = %q, want /built/client", s.Client.BinaryPath)
	}
}

// persistClientBuildCase drives one scenario of the shared client-build
// persistence helper.
type persistClientBuildCase struct {
	name      string
	success   bool
	blockSave bool
	wantSaved bool
	wantWarn  bool
}

// TestPersistClientBuildRecordsAndWarns covers persistClientBuild: a
// successful build records the client in state, a persistence failure appends
// the warning instead of failing the response, and a failed build records
// nothing.
func TestPersistClientBuildRecordsAndWarns(t *testing.T) {
	tests := []persistClientBuildCase{
		{name: "success saves the client", success: true, wantSaved: true},
		{name: "failed build records nothing", success: false, wantSaved: false},
		{name: "persist failure warns", success: true, blockSave: true, wantWarn: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runPersistClientBuildCase(t, tt)
		})
	}
}

func runPersistClientBuildCase(t *testing.T, tt persistClientBuildCase) {
	t.Helper()
	t.Chdir(t.TempDir())
	origProfile := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(origProfile) })
	if tt.blockSave {
		if err := os.WriteFile(".ludus", []byte("not a directory"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result := gameBuildResult{Success: tt.success, Binary: "/built/client", OutputDir: "/built"}
	persistClientBuild(false, "Linux", "hash", &result)

	gotWarn := strings.Contains(result.Output, ".ludus state could not be updated")
	if gotWarn != tt.wantWarn {
		t.Errorf("result.Output = %q, want warning = %v", result.Output, tt.wantWarn)
	}
	if tt.blockSave {
		return // the blocked state file cannot be loaded back
	}
	assertSavedClientState(t, tt.wantSaved)
}

// TestApplyBuildResultHelpersNil covers the nil-tolerance of the result
// assembly helpers: a builder returning no result must not touch the fields.
func TestApplyBuildResultHelpersNil(t *testing.T) {
	var result gameBuildResult
	applyServerBuildResult(nil, &result)
	applyClientBuildResult(nil, &result)
	if result.Success || result.Binary != "" || result.DurationSeconds != 0 {
		t.Errorf("nil build result must not populate fields, got %+v", result)
	}
}
