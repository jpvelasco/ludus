package root

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
)

func captureRootStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	runErr := run()
	os.Stdout = previous
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}

func TestRunInitReportsFailedChecks(t *testing.T) {
	setRootGlobals(t)
	previousJSON := globals.JSONOutput
	globals.JSONOutput = false
	t.Cleanup(func() { globals.JSONOutput = previousJSON })
	t.Setenv("PATH", t.TempDir()) // every exec-based check degrades via LookPath

	output, err := captureRootStdout(t, func() error { return runInit(initCmd, nil) })
	if err == nil {
		t.Fatal("runInit() expected error when prerequisites fail")
	}
	if !strings.Contains(output, "Validating prerequisites...") {
		t.Errorf("output %q missing header line", output)
	}
	if !strings.Contains(output, "[FAIL]") {
		t.Errorf("output %q missing [FAIL] markers", output)
	}
}

func TestRunInitJSONOutput(t *testing.T) {
	setRootGlobals(t)
	previousJSON := globals.JSONOutput
	globals.JSONOutput = true
	t.Cleanup(func() { globals.JSONOutput = previousJSON })
	t.Setenv("PATH", t.TempDir())

	output, err := captureRootStdout(t, func() error { return runInit(initCmd, nil) })
	if err != nil {
		t.Fatalf("runInit() JSON error = %v", err)
	}
	if !strings.Contains(output, `"passed"`) {
		t.Errorf("output %q missing JSON passed field", output)
	}
	if strings.Contains(output, "Validating prerequisites...") {
		t.Errorf("output %q should not contain human-readable header in JSON mode", output)
	}
}
