package testsupport

import (
	"os"
	"testing"
)

// ToolBehavior defines the behavior of a fake tool.
type ToolBehavior struct {
	ExitCode int    // Exit code to return
	Stdout   string // Text to print to stdout
	Stderr   string // Text to print to stderr
}

// FakeTool creates a fake executable tool in a temporary directory, adds it to PATH,
// and returns the path to the tool. The tool can be configured with exit code, stdout, and stderr.
// Use on Windows will create a .bat file; on Unix, a shell script.
func FakeTool(t *testing.T, name string, behavior ToolBehavior) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return createFakeTool(t, dir, name, behavior)
}
