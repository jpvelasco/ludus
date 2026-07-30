//go:build !windows

package testsupport

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// createFakeTool creates a fake shell script with the specified behavior.
func createFakeTool(t *testing.T, dir, name string, behavior ToolBehavior) string {
	t.Helper()
	path := filepath.Join(dir, name)

	// Build shell script that outputs to stdout/stderr and exits with the given code
	script := "#!/bin/sh\n"
	if behavior.Stdout != "" {
		script += fmt.Sprintf("printf '%%s\\n' %q\n", behavior.Stdout)
	}
	if behavior.Stderr != "" {
		script += fmt.Sprintf("printf '%%s\\n' %q >&2\n", behavior.Stderr)
	}
	if behavior.ExitCode != 0 {
		script += fmt.Sprintf("exit %d\n", behavior.ExitCode)
	}

	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	return path
}
