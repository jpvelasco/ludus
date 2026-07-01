package status

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	internalstatus "github.com/jpvelasco/ludus/internal/status"
	"github.com/spf13/cobra"
)

func TestRunStatusHumanOutput(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	outputDir := filepath.Join(tempDir, "export")
	if err := os.Mkdir(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "server"), []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}

	setStatusGlobals(t, &config.Config{
		Game:   config.GameConfig{ProjectName: "Example"},
		Deploy: config.DeployConfig{Target: "binary", OutputDir: outputDir},
	}, false)

	stdout, stderr, err := captureStatusOutput(t, func() error {
		return runStatus(commandWithContext(), nil)
	})
	if err != nil {
		t.Fatalf("runStatus() error = %v", err)
	}
	for _, want := range []string{"Pipeline Status", "[OK]", "Binary Deployment", "[FAIL]", "Engine Source"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestRunStatusJSONOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	setStatusGlobals(t, &config.Config{
		Game:   config.GameConfig{ProjectName: "Example"},
		Deploy: config.DeployConfig{Target: "binary", OutputDir: "missing"},
	}, true)

	stdout, stderr, err := captureStatusOutput(t, func() error {
		return runStatus(commandWithContext(), nil)
	})
	if err != nil {
		t.Fatalf("runStatus() error = %v", err)
	}

	var stages []internalstatus.StageStatus
	if err := json.Unmarshal([]byte(stdout), &stages); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput: %s", err, stdout)
	}
	if len(stages) != 7 {
		t.Errorf("len(stages) = %d, want 7", len(stages))
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestRunStatusReportsTargetResolutionWarning(t *testing.T) {
	t.Chdir(t.TempDir())
	setStatusGlobals(t, &config.Config{
		Deploy: config.DeployConfig{Target: "unsupported"},
	}, false)

	stdout, stderr, err := captureStatusOutput(t, func() error {
		return runStatus(commandWithContext(), nil)
	})
	if err != nil {
		t.Fatalf("runStatus() error = %v", err)
	}
	if !strings.Contains(stderr, `unknown deploy target "unsupported"`) {
		t.Errorf("stderr = %q, want target resolution warning", stderr)
	}
	if !strings.Contains(stdout, "target not resolved") {
		t.Errorf("stdout = %q, want unresolved target detail", stdout)
	}
}

func commandWithContext() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

func setStatusGlobals(t *testing.T, cfg *config.Config, jsonOutput bool) {
	t.Helper()
	previousCfg := globals.Cfg
	previousJSON := globals.JSONOutput
	globals.Cfg = cfg
	globals.JSONOutput = jsonOutput
	t.Cleanup(func() {
		globals.Cfg = previousCfg
		globals.JSONOutput = previousJSON
	})
}

func captureStatusOutput(t *testing.T, run func() error) (string, string, error) {
	t.Helper()
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previousStdout, previousStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter

	runErr := run()
	os.Stdout, os.Stderr = previousStdout, previousStderr
	if err := stdoutWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stderrWriter.Close(); err != nil {
		t.Fatal(err)
	}
	stdout := readStatusPipe(t, stdoutReader)
	stderr := readStatusPipe(t, stderrReader)
	return stdout, stderr, runErr
}

func readStatusPipe(t *testing.T, reader *os.File) string {
	t.Helper()
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
