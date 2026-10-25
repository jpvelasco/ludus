package game

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	gameBuilder "github.com/jpvelasco/ludus/internal/game"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// savedClientFixture is the valid client state a dry-run preview must
// preserve (#733): its binary was built by a previous real build.
var savedClientFixture = &state.ClientState{
	BinaryPath: "/real/builds/Lyra.sh",
	OutputDir:  "/real/builds",
	Platform:   "Linux",
	BuiltAt:    "2026-01-01T00:00:00Z",
}

// seedSavedClientFixture writes a valid saved client into a fresh working
// directory so a later dry-run save can be observed against it.
func seedSavedClientFixture(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	origProfile := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(origProfile) })
	if err := state.UpdateClient(savedClientFixture); err != nil {
		t.Fatalf("seeding saved client state: %v", err)
	}
}

// assertClientStatePreserved verifies the saved client survived a dry-run
// build (#733).
func assertClientStatePreserved(t *testing.T) {
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
		"binary":   {got: s.Client.BinaryPath, want: savedClientFixture.BinaryPath},
		"output":   {got: s.Client.OutputDir, want: savedClientFixture.OutputDir},
		"platform": {got: s.Client.Platform, want: savedClientFixture.Platform},
		"builtAt":  {got: s.Client.BuiltAt, want: savedClientFixture.BuiltAt},
	}
	for name, field := range fields {
		if field.got != field.want {
			t.Errorf("saved client %s overwritten by a dry-run build: got %q, want %q", name, field.got, field.want)
		}
	}
}

// clientBuildCfg returns a config whose native dry-run client build succeeds
// and whose planned output differs from the saved fixture, with the client
// flags reset for the duration of the test.
func clientBuildCfg(t *testing.T) *config.Config {
	t.Helper()
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "TestGame")
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: engineRoot, Version: "5.7.3", Backend: "native"},
		Game:   config.GameConfig{ProjectName: "TestGame", ProjectPath: projectPath, Platform: "Linux", Arch: "amd64"},
	}
	origPlatform, origNoCache := clientPlatform, noCacheClient
	clientPlatform, noCacheClient = "Linux", true
	t.Cleanup(func() { clientPlatform, noCacheClient = origPlatform, origNoCache })
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithDDCMode("none"))
	return cfg
}

// TestSaveClientStateSkippedUnderDryRun pins #733 at the save helper: a
// dry-run preview must not overwrite the saved client.
func TestSaveClientStateSkippedUnderDryRun(t *testing.T) {
	seedSavedClientFixture(t)
	globals.SetGlobals(t, &config.Config{}, globals.WithDryRun(true))

	saveClientState(&gameBuilder.ClientBuildResult{
		ClientBinary: "/planned/different/Lyra",
		OutputDir:    "/planned/different",
		Platform:     "Linux",
	})

	assertClientStatePreserved(t)
}

// TestRunClientBuildDryRunPreservesSavedClient pins #733 at the command: a
// full dry-run native client build reports success without touching the
// saved client state.
func TestRunClientBuildDryRunPreservesSavedClient(t *testing.T) {
	seedSavedClientFixture(t)
	clientBuildCfg(t)
	// noCacheClient=true (set by clientBuildCfg) forces a cache miss so the
	// dry-run build always runs.

	output := captureGameStdout(t, func() {
		if err := runClientBuild(gameCmd(t), nil); err != nil {
			t.Fatalf("runClientBuild(dry-run) error = %v", err)
		}
	})
	if !strings.Contains(output, "client build complete") {
		t.Errorf("runClientBuild(dry-run) output = %q, want the success message", output)
	}

	assertClientStatePreserved(t)
}
