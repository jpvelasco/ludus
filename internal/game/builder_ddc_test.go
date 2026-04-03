package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
)

func TestDDCArgs(t *testing.T) {
	tests := []struct {
		name    string
		opts    BuildOptions
		wantNil bool
		wantErr bool
		errMsg  string
	}{
		{
			name:    "mode none returns nil",
			opts:    BuildOptions{DDCMode: "none", DDCPath: "/some/path"},
			wantNil: true,
		},
		{
			name:    "empty mode returns nil",
			opts:    BuildOptions{DDCMode: "", DDCPath: "/some/path"},
			wantNil: true,
		},
		{
			name:    "local with empty path errors",
			opts:    BuildOptions{DDCMode: "local", DDCPath: ""},
			wantErr: true,
			errMsg:  "no path configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(tt.opts, &runner.Runner{})
			args, err := b.ddcArgs()
			if tt.wantErr {
				if err == nil {
					t.Fatal("ddcArgs() should have returned an error")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error should contain %q, got: %v", tt.errMsg, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ddcArgs() unexpected error: %v", err)
			}
			if tt.wantNil && args != nil {
				t.Errorf("ddcArgs() = %v, want nil", args)
			}
		})
	}
}

func TestDDCArgs_LocalWithPath(t *testing.T) {
	ddcDir := filepath.Join(t.TempDir(), "ddc")
	b := NewBuilder(BuildOptions{DDCMode: "local", DDCPath: ddcDir}, &runner.Runner{})
	args, err := b.ddcArgs()
	if err != nil {
		t.Fatalf("ddcArgs() error: %v", err)
	}

	if len(args) != 2 {
		t.Fatalf("ddcArgs() returned %d args, want 2", len(args))
	}

	if _, err := os.Stat(ddcDir); err != nil {
		t.Errorf("DDC directory not created: %v", err)
	}

	for i, arg := range args {
		if !strings.Contains(arg, "-ini:Engine:") {
			t.Errorf("args[%d] should be -ini: override, got: %s", i, arg)
		}
	}
}

func TestDDCArgs_CreatesNestedDirectory(t *testing.T) {
	ddcDir := filepath.Join(t.TempDir(), "nested", "ddc")
	b := NewBuilder(BuildOptions{DDCMode: "local", DDCPath: ddcDir}, &runner.Runner{})
	args, err := b.ddcArgs()
	if err != nil {
		t.Fatalf("ddcArgs() error: %v", err)
	}

	if args == nil {
		t.Fatal("ddcArgs() returned nil, want args")
	}
	if _, err := os.Stat(ddcDir); err != nil {
		t.Errorf("DDC directory not created (nested): %v", err)
	}
}
