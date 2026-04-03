package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
)

func TestDDCArgs_ModeNone(t *testing.T) {
	b := NewBuilder(BuildOptions{DDCMode: "none", DDCPath: "/some/path"}, &runner.Runner{})
	args := b.ddcArgs()
	if args != nil {
		t.Errorf("ddcArgs() = %v, want nil for mode=none", args)
	}
}

func TestDDCArgs_EmptyMode(t *testing.T) {
	b := NewBuilder(BuildOptions{DDCMode: "", DDCPath: "/some/path"}, &runner.Runner{})
	args := b.ddcArgs()
	if args != nil {
		t.Errorf("ddcArgs() = %v, want nil for empty mode", args)
	}
}

func TestDDCArgs_EmptyPath(t *testing.T) {
	b := NewBuilder(BuildOptions{DDCMode: "local", DDCPath: ""}, &runner.Runner{})
	args := b.ddcArgs()
	if args != nil {
		t.Errorf("ddcArgs() = %v, want nil for empty path", args)
	}
}

func TestDDCArgs_LocalWithPath(t *testing.T) {
	ddcDir := filepath.Join(t.TempDir(), "ddc")
	b := NewBuilder(BuildOptions{DDCMode: "local", DDCPath: ddcDir}, &runner.Runner{})
	args := b.ddcArgs()

	if len(args) != 2 {
		t.Fatalf("ddcArgs() returned %d args, want 2", len(args))
	}

	// Verify DDC directory was created
	if _, err := os.Stat(ddcDir); err != nil {
		t.Errorf("DDC directory not created: %v", err)
	}

	// Verify args are -ini: overrides
	for i, arg := range args {
		if !strings.Contains(arg, "-ini:Engine:") {
			t.Errorf("args[%d] should be -ini: override, got: %s", i, arg)
		}
	}
}

func TestDDCArgs_CreatesNestedDirectory(t *testing.T) {
	ddcDir := filepath.Join(t.TempDir(), "nested", "ddc")
	b := NewBuilder(BuildOptions{DDCMode: "local", DDCPath: ddcDir}, &runner.Runner{})
	args := b.ddcArgs()

	if args == nil {
		t.Fatal("ddcArgs() returned nil, want args")
	}
	if _, err := os.Stat(ddcDir); err != nil {
		t.Errorf("DDC directory not created (nested): %v", err)
	}
}
