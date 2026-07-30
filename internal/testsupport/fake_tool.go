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
//
// Each call prepends a fresh directory, so calling FakeTool twice leaves both
// tools resolvable. Use FakeTools when stubbing a set of tools together.
func FakeTool(t *testing.T, name string, behavior ToolBehavior) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return createFakeTool(t, dir, name, behavior)
}

// FakeTools stubs several executables at once in a single temporary directory
// prepended to PATH. Use it when code under test looks up more than one external
// tool, so every stub stays resolvable from the same PATH entry.
func FakeTools(t *testing.T, tools map[string]ToolBehavior) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for name, behavior := range tools {
		createFakeTool(t, dir, name, behavior)
	}
	return dir
}
