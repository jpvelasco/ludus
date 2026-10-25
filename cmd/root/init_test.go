package root

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/prereq"
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

// TestRunInitJSONOutputFailsWithResults pins the #748 fix: JSON automation
// gets the full check results on stdout AND a nonzero exit when checks fail,
// matching human-mode validation semantics.
func TestRunInitJSONOutputFailsWithResults(t *testing.T) {
	setRootGlobals(t)
	previousJSON := globals.JSONOutput
	globals.JSONOutput = true
	t.Cleanup(func() { globals.JSONOutput = previousJSON })
	t.Setenv("PATH", t.TempDir()) // every exec-based check fails

	output, err := captureRootStdout(t, func() error { return runInit(initCmd, nil) })
	if err == nil {
		t.Fatal("runInit() expected an error when prerequisites fail in JSON mode")
	}
	if !strings.Contains(err.Error(), "prerequisite check(s) failed") {
		t.Errorf("error = %v, want failed-checks message", err)
	}
	if !strings.Contains(output, `"passed"`) {
		t.Errorf("output %q missing JSON passed field", output)
	}
	if !strings.Contains(output, `"passed": false`) {
		t.Errorf("output %q should include the failed checks", output)
	}
	if strings.Contains(output, "Validating prerequisites...") {
		t.Errorf("output %q should not contain human-readable header in JSON mode", output)
	}
}

// TestRunInitJSONEncodeError covers the failure path of the JSON encoder
// itself: the command reports the encoding error instead of exiting zero.
func TestRunInitJSONEncodeError(t *testing.T) {
	setRootGlobals(t)
	previousJSON := globals.JSONOutput
	globals.JSONOutput = true
	t.Cleanup(func() { globals.JSONOutput = previousJSON })
	t.Setenv("PATH", t.TempDir())

	// A read-only stdout makes json.Encoder.Encode fail on the first write.
	readOnly, err := os.Open("init.go")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { readOnly.Close() })
	previous := os.Stdout
	os.Stdout = readOnly
	runErr := runInit(initCmd, nil)
	os.Stdout = previous

	if runErr == nil || !strings.Contains(runErr.Error(), "encoding results") {
		t.Fatalf("runInit() error = %v, want encoding error", runErr)
	}
}

// TestReportInitJSONAllPassed covers the JSON success path: passing results
// encode cleanly and the command reports success.
func TestReportInitJSONAllPassed(t *testing.T) {
	results := []prereq.CheckResult{{Name: "Toolchain", Passed: true, Message: "clang 20"}}

	output, err := captureRootStdout(t, func() error { return reportInitJSON(results, 0) })
	if err != nil {
		t.Fatalf("reportInitJSON() error = %v, want nil", err)
	}
	if !strings.Contains(output, `"passed": true`) {
		t.Errorf("output %q should include the passing check", output)
	}
}
