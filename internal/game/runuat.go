package game

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// resolveRunUAT returns the shell command and RunUAT script path for the current OS.
// On Windows: cmd, RunUAT.bat; on Linux/macOS: bash, RunUAT.sh.
// The returned scriptPath is relative to the engine root (used with RunInDir).
func (b *Builder) resolveRunUAT() (shell, scriptPath string, err error) {
	relPath := filepath.Join("Engine", "Build", "BatchFiles")
	absCheck := filepath.Join(b.opts.EnginePath, relPath)
	if runtime.GOOS == "windows" {
		shell = "cmd"
		scriptPath = filepath.Join(relPath, "RunUAT.bat")
		absCheck = filepath.Join(absCheck, "RunUAT.bat")
	} else {
		shell = "bash"
		scriptPath = filepath.Join(relPath, "RunUAT.sh")
		absCheck = filepath.Join(absCheck, "RunUAT.sh")
	}
	if _, statErr := os.Stat(absCheck); os.IsNotExist(statErr) {
		return "", "", fmt.Errorf("%s not found at %s", filepath.Base(absCheck), absCheck)
	}
	return shell, scriptPath, nil
}

// execRunUAT runs RunUAT with the given arguments using the appropriate shell for the OS.
// scriptPath is relative to the engine root directory (set via RunInDir).
func (b *Builder) execRunUAT(ctx context.Context, shell, scriptPath string, uatArgs []string) error {
	var args []string
	if runtime.GOOS == "windows" {
		args = append([]string{"/c", scriptPath}, uatArgs...)
	} else {
		args = append([]string{scriptPath}, uatArgs...)
	}
	return b.Runner.RunInDir(ctx, b.opts.EnginePath, shell, args...)
}
