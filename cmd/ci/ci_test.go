package ci

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestRunInitDryRunUsesDefaults(t *testing.T) {
	resetCIGlobals(t)
	globals.Cfg = &config.Config{}
	globals.DryRun = true

	output, err := captureCIStdout(t, func() error { return runInit(initCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"runs-on: [self-hosted, linux, x64]",
		"# push:",
		"# pull_request:",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output does not contain %q", want)
		}
	}
}

func TestRunInitWritesConfiguredWorkflow(t *testing.T) {
	resetCIGlobals(t)
	path := filepath.Join(t.TempDir(), "nested", "workflow.yml")
	globals.Cfg = &config.Config{}
	globals.Cfg.CI.WorkflowPath = path
	globals.Cfg.CI.RunnerLabels = []string{"self-hosted", "windows", "ue5"}
	enablePush = true
	enablePR = true

	output, err := captureCIStdout(t, func() error { return runInit(initCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"runs-on: [self-hosted, windows, ue5]",
		"  push:",
		"  pull_request:",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("workflow does not contain %q", want)
		}
	}
	if !strings.Contains(output, "Workflow written to "+path) {
		t.Fatalf("output = %q, want written path", output)
	}
}

func TestRunnerResolvers(t *testing.T) {
	resetCIGlobals(t)
	globals.Cfg = &config.Config{}
	globals.Cfg.CI.RunnerDir = "/configured/runner"
	globals.Cfg.CI.RunnerLabels = []string{"self-hosted", "linux", "arm64"}

	tests := []struct {
		name string
		set  func()
		got  func() string
		want string
	}{
		{"configured directory", func() {}, resolveDir, "/configured/runner"},
		{"directory override", func() { runnerDir = "/flag/runner" }, resolveDir, "/flag/runner"},
		{"configured labels", func() {}, resolveLabels, "self-hosted,linux,arm64"},
		{"label override", func() { runnerLabels = "custom,gpu" }, resolveLabels, "custom,gpu"},
		{"name override", func() { runnerName = "build-host" }, resolveName, "build-host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runnerDir, runnerLabels, runnerName = "", "", ""
			tt.set()
			if got := tt.got(); got != tt.want {
				t.Fatalf("result = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveLabelsDefault(t *testing.T) {
	resetCIGlobals(t)
	globals.Cfg = &config.Config{}
	if got, want := resolveLabels(), "self-hosted,linux,x64"; got != want {
		t.Fatalf("resolveLabels() = %q, want %q", got, want)
	}
}

func TestResolveRepoExplicit(t *testing.T) {
	resetCIGlobals(t)
	runnerRepo = "owner/repository"
	got, err := resolveRepo(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != runnerRepo {
		t.Fatalf("resolveRepo() = %q, want %q", got, runnerRepo)
	}
}

func resetCIGlobals(t *testing.T) {
	t.Helper()
	previousCfg, previousDryRun := globals.Cfg, globals.DryRun
	previousOutput, previousPush, previousPR := outputPath, enablePush, enablePR
	previousDir, previousLabels := runnerDir, runnerLabels
	previousName, previousRepo := runnerName, runnerRepo
	globals.DryRun = false
	outputPath, enablePush, enablePR = "", false, false
	runnerDir, runnerLabels, runnerName, runnerRepo = "", "", "", ""
	t.Cleanup(func() {
		globals.Cfg, globals.DryRun = previousCfg, previousDryRun
		outputPath, enablePush, enablePR = previousOutput, previousPush, previousPR
		runnerDir, runnerLabels = previousDir, previousLabels
		runnerName, runnerRepo = previousName, previousRepo
	})
}

func captureCIStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	runErr := run()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previous
	t.Cleanup(func() { os.Stdout = previous })
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}
