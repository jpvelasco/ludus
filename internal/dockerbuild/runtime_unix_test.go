//go:build !windows

package dockerbuild

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}

func TestResolvePodmanFallback_NonWindows(t *testing.T) {
	if got := ResolvePodmanFallback(); got != "" {
		t.Errorf("ResolvePodmanFallback() = %q, want empty", got)
	}
}
