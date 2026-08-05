package ddc

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	internalddc "github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestRunWarmupResolveDDCError(t *testing.T) {
	globals.SetGlobals(t, &config.Config{}, globals.WithDDCMode("bogus"))

	err := runWarmup(warmupCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid DDC mode") {
		t.Fatalf("runWarmup() error = %v, want invalid mode error", err)
	}
}

func TestRunWarmupDryRunPreview(t *testing.T) {
	zenPath := filepath.Join(t.TempDir(), "zen")
	cfg := &config.Config{
		DDC: config.DDCConfig{
			Mode:    internalddc.ModeZen,
			ZenPath: zenPath,
		},
		Game: config.GameConfig{
			ProjectPath: filepath.Join(t.TempDir(), "Fake.uproject"),
		},
		Engine: config.EngineConfig{
			DockerImage: "fake-engine:5.7.3",
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithDDCMode(internalddc.ModeZen))

	output, err := captureDDCStdout(t, func() error { return runWarmup(warmupCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"DRY RUN: DDC Warmup", "Image  : fake-engine:5.7.3", "Project: " + cfg.Game.ProjectPath, "DDC    : " + zenPath} {
		if !strings.Contains(output, want) {
			t.Errorf("output %q does not contain %q", output, want)
		}
	}
}

func TestRunWarmupDryRunPreviewImageError(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := warmupConfig(t, "docker", "")
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithDDCMode(internalddc.ModeZen))

	err := runWarmup(warmupCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "could not detect engine version") {
		t.Fatalf("runWarmup() error = %v, want engine version error", err)
	}
}

func TestRunWarmupNonContainerBackend(t *testing.T) {
	cfg := warmupConfig(t, "native", "5.7.3")
	globals.SetGlobals(t, cfg, globals.WithDDCMode(internalddc.ModeZen), globals.WithNoLogs(true))

	err := runWarmup(warmupCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "requires a container backend") {
		t.Fatalf("runWarmup() error = %v, want container backend error", err)
	}
}

func TestRunWarmupUndetectableEngineVersion(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := warmupConfig(t, "docker", "")
	globals.SetGlobals(t, cfg, globals.WithDDCMode(internalddc.ModeZen), globals.WithNoLogs(true))

	err := runWarmup(warmupCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "could not detect engine version") {
		t.Fatalf("runWarmup() error = %v, want engine version error", err)
	}
}

func TestExecuteWarmupDockerFailure(t *testing.T) {
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 1})
	cfg := warmupConfig(t, "docker", "5.7.3")
	globals.SetGlobals(t, cfg, globals.WithDDCMode(internalddc.ModeZen), globals.WithNoLogs(true))
	withWarmupContext(t)

	err := runWarmup(warmupCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "DDC warmup failed") {
		t.Fatalf("runWarmup() error = %v, want warmup failure error", err)
	}
}

func TestExecuteWarmupDockerSuccess(t *testing.T) {
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 0})
	cfg := warmupConfig(t, "docker", "5.7.3")
	globals.SetGlobals(t, cfg, globals.WithDDCMode(internalddc.ModeZen), globals.WithNoLogs(true))
	withWarmupContext(t)

	output, err := captureDDCStdout(t, func() error { return runWarmup(warmupCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "DDC warmup complete.") {
		t.Errorf("output %q does not contain warmup completion", output)
	}
}

// withWarmupContext gives the shared warmupCmd a valid Context for the duration
// of the test, since runWarmup passes cmd.Context() down to the container build.
func withWarmupContext(t *testing.T) {
	t.Helper()
	warmupCmd.SetContext(context.Background())
	t.Cleanup(func() { warmupCmd.SetContext(context.Background()) })
}

func warmupConfig(t *testing.T, backend, version string) *config.Config {
	t.Helper()
	return &config.Config{
		DDC: config.DDCConfig{
			Mode:      internalddc.ModeZen,
			ZenPath:   filepath.Join(t.TempDir(), "zen"),
			LocalPath: filepath.Join(t.TempDir(), "ddc"),
		},
		Game: config.GameConfig{
			ProjectPath: filepath.Join(t.TempDir(), "Fake.uproject"),
		},
		Engine: config.EngineConfig{
			Backend: backend,
			Version: version,
		},
	}
}
