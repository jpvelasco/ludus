package game

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	gameBuilder "github.com/jpvelasco/ludus/internal/game"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestPrintBuildConfigGuidance(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		want string
	}{
		{name: "shipping", cfg: "Shipping", want: "Shipping (optimized"},
		{name: "development", cfg: "Development", want: "Development (debug symbols"},
		{name: "empty", cfg: "", want: ""},
		{name: "custom", cfg: "Test", want: "Build config: Test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := captureGameStdout(t, func() { printBuildConfigGuidance(tt.cfg) })
			if tt.want == "" {
				if got != "" {
					t.Errorf("output = %q, want no output", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("output = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestNextAfterServerBuild(t *testing.T) {
	original := globals.Cfg
	t.Cleanup(func() { globals.Cfg = original })
	tests := []struct {
		target string
		want   string
	}{
		{target: "gamelift", want: "ludus container build"},
		{target: "STACK", want: "ludus container build"},
		{target: "ec2", want: "ludus deploy ec2"},
		{target: "anywhere", want: "ludus deploy anywhere"},
		{target: "binary", want: "ludus deploy binary"},
		{target: "", want: "ludus container build  (or: ludus deploy <target>)"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			globals.Cfg = &config.Config{}
			globals.Cfg.Deploy.Target = tt.target
			if got := nextAfterServerBuild(); got != tt.want {
				t.Errorf("nextAfterServerBuild() = %q, want %q", got, tt.want)
			}
		})
	}
}

// seedServerOutput creates the packaged server directory for cfg so a cache
// hit passes its output validation (#739).
func seedServerOutput(t *testing.T, cfg *config.Config) {
	t.Helper()
	if err := os.MkdirAll(config.ResolveServerBuildDir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestResolvedBuildConfigAppliesBackendOverride pins the CLI half of #728:
// resolvedBuildConfig must carry the effective --backend (flag over config)
// so EngineKey hashes the backend that will actually run, instead of the raw
// YAML value captured before flag resolution.
func TestResolvedBuildConfigAppliesBackendOverride(t *testing.T) {
	originalCfg, originalBackend := globals.Cfg, backend
	t.Cleanup(func() {
		globals.Cfg, backend = originalCfg, originalBackend
	})

	globals.Cfg = &config.Config{}
	globals.Cfg.Engine.Backend = "native"
	backend = "wsl2"

	cfg := resolvedBuildConfig()
	if cfg.Engine.Backend != "wsl2" {
		t.Fatalf("resolved build backend = %q, want wsl2", cfg.Engine.Backend)
	}
	if got := cache.EngineKey(&cfg); got == cache.EngineKey(globals.Cfg) {
		t.Error("cache key should change when --backend overrides the configured backend")
	}
}

// TestRunBuildBuildConfigChangesCacheKey pins #696 at the CLI entry point: a
// Shipping request must not reuse a Development-warmed cache entry, while an
// explicit Development request must.
func TestRunBuildBuildConfigChangesCacheKey(t *testing.T) {
	tests := []struct {
		name        string
		request     string // --build-config of the request
		seed        string // --build-config the cache was warmed with
		wantSkipped bool
	}{
		{name: "shipping request misses development cache", request: "Shipping", seed: "", wantSkipped: false},
		{name: "explicit development reuses default cache", request: "Development", seed: "", wantSkipped: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
			projectPath := testsupport.FakeProject(t, "TestGame")

			cfg := &config.Config{
				Engine: config.EngineConfig{SourcePath: engineRoot, Version: "5.7.3", Backend: "native"},
				Game:   config.GameConfig{ProjectName: "TestGame", ProjectPath: projectPath, Platform: "Linux", Arch: "amd64"},
			}
			globals.SetGlobals(t, cfg, globals.WithDryRun(true))

			origConfig, origNoCache := serverConfig, noCache
			serverConfig, noCache = tt.seed, false
			t.Cleanup(func() { serverConfig, noCache = origConfig, origNoCache })

			seedCfg := resolvedBuildConfig()
			seedGameCache(t, cache.StageGameServer,
				cache.GameServerKey(&seedCfg, cache.EngineKey(&seedCfg), tt.seed))
			seedServerOutput(t, &seedCfg)

			serverConfig = tt.request
			output := captureGameStdout(t, func() {
				if err := runBuild(gameCmd(t), nil); err != nil {
					t.Fatalf("runBuild() error = %v, want nil", err)
				}
			})
			if skipped := strings.Contains(output, "(cached), skipping"); skipped != tt.wantSkipped {
				t.Errorf("cache skip = %v, want %v (output: %s)", skipped, tt.wantSkipped, output)
			}
		})
	}
}

// TestRunBuildBackendFlagChangesCacheKey pins the dispatch-level half of #728:
// a request whose --backend differs from the warmed entry must rebuild instead
// of skipping under the config backend's key.
func TestRunBuildBackendFlagChangesCacheKey(t *testing.T) {
	projectPath := testsupport.FakeProject(t, "TestGame")

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "C:/ue5",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
			Backend:     "native",
		},
		Game: config.GameConfig{ProjectName: "TestGame", ProjectPath: projectPath, Platform: "Linux", Arch: "amd64"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	// Warm the cache under the CONFIG backend (native) with the flag unset,
	// then request the container backend via the flag.
	origBackend := backend
	backend = ""
	t.Cleanup(func() { backend = origBackend })

	seedCfg := resolvedBuildConfig()
	seedGameCache(t, cache.StageGameServer,
		cache.GameServerKey(&seedCfg, cache.EngineKey(&seedCfg), ""))
	seedServerOutput(t, &seedCfg)

	backend = "docker"
	output := captureGameStdout(t, func() {
		if err := runBuild(gameCmd(t), nil); err != nil {
			t.Fatalf("runBuild() error = %v, want nil", err)
		}
	})
	if strings.Contains(output, "(cached), skipping") {
		t.Errorf("docker request reused the native-warmed cache entry: %s", output)
	}
}

func TestResolveGameBackendAndArch(t *testing.T) {
	originalCfg, originalBackend, originalArch := globals.Cfg, backend, archFlag
	t.Cleanup(func() {
		globals.Cfg, backend, archFlag = originalCfg, originalBackend, originalArch
	})
	globals.Cfg = &config.Config{}
	globals.Cfg.Engine.Backend = "docker"
	globals.Cfg.Game.Arch = "amd64"
	backend, archFlag = "", ""
	if got := resolveBackend(); got != "docker" {
		t.Errorf("configured backend = %q, want docker", got)
	}
	if got := resolveArch(); got != "amd64" {
		t.Errorf("configured arch = %q, want amd64", got)
	}
	backend, archFlag = "podman", "aarch64"
	if got := resolveBackend(); got != "podman" {
		t.Errorf("flag backend = %q, want podman", got)
	}
	if got := resolveArch(); got != "arm64" {
		t.Errorf("flag arch = %q, want arm64", got)
	}
}

func TestSaveClientState(t *testing.T) {
	t.Chdir(t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })
	saveClientState(&gameBuilder.ClientBuildResult{
		ClientBinary: "build/MyGame",
		OutputDir:    "build",
		Platform:     "Linux",
	})
	got, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	if got.Client == nil {
		t.Fatal("client state is nil")
	}
	fields := map[string]struct {
		got  string
		want string
	}{
		"binary":   {got: got.Client.BinaryPath, want: "build/MyGame"},
		"output":   {got: got.Client.OutputDir, want: "build"},
		"platform": {got: got.Client.Platform, want: "Linux"},
	}
	for name, field := range fields {
		if field.got != field.want {
			t.Errorf("%s = %q, want %q", name, field.got, field.want)
		}
	}
	if got.Client.BuiltAt == "" {
		t.Error("BuiltAt is empty")
	}
}

func captureGameStdout(t *testing.T, fn func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = previous })
	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	os.Stdout = previous
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	return string(output)
}

// blockStateWrite makes .ludus/state.json unwritable by placing a file where
// the state directory should be, so state.UpdateClient fails and the warning
// branch of saveClientState runs.
func blockStateWrite(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".ludus", []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestSaveClientState_WarnOnWriteFailure covers the state-write failure branch
// of saveClientState (game.go:28-30): the function must print a warning and not
// panic when the state file cannot be written.
func TestSaveClientState_WarnOnWriteFailure(t *testing.T) {
	previous := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(previous) })
	blockStateWrite(t)

	output := captureGameStdout(t, func() {
		saveClientState(&gameBuilder.ClientBuildResult{
			ClientBinary: "build/MyGame",
			OutputDir:    "build",
			Platform:     "Linux",
		})
	})

	if !strings.Contains(output, "Warning: failed to write state") {
		t.Errorf("output = %q, want a 'Warning: failed to write state' message", output)
	}
}
