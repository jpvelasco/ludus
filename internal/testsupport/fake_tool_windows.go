//go:build windows

package testsupport

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// createFakeTool creates a fake .bat file with the specified behavior.
func createFakeTool(t *testing.T, dir, name string, behavior ToolBehavior) string {
	t.Helper()
	path := filepath.Join(dir, name+".bat")

	// Build batch script that outputs to stdout/stderr and exits with the given code
	script := "@echo off\n"
	if behavior.Stdout != "" {
		// Escape quotes in stdout for batch context
		escaped := behavior.Stdout
		script += fmt.Sprintf("echo %s\n", escaped)
	}
	if behavior.Stderr != "" {
		// Route to stderr using (>&2)
		escaped := behavior.Stderr
		script += fmt.Sprintf("echo %s 1>&2\n", escaped)
	}
	if behavior.ExitCode != 0 {
		script += fmt.Sprintf("exit /b %d\n", behavior.ExitCode)
	}

	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}
