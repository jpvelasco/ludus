package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/runner"
)

func TestSetupDDC(t *testing.T) {
	tests := []struct {
		name    string
		opts    BuildOptions
		wantEnv bool
		wantErr bool
		errMsg  string
	}{
		{
			name:    "zen mode is a no-op (UE persists its own Zen Store natively)",
			opts:    BuildOptions{DDCMode: "zen", DDCPath: "/some/path"},
			wantEnv: false,
		},
		{
			name:    "mode none returns nil",
			opts:    BuildOptions{DDCMode: "none", DDCPath: "/some/path"},
			wantEnv: false,
		},
		{
			name:    "empty mode returns nil",
			opts:    BuildOptions{DDCMode: "", DDCPath: "/some/path"},
			wantEnv: false,
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
			r := runner.NewRunner(false, false)
			b := NewBuilder(tt.opts, r)
			err := b.setupDDC()
			if tt.wantErr {
				if err == nil {
					t.Fatal("setupDDC() should have returned an error")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error should contain %q, got: %v", tt.errMsg, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("setupDDC() unexpected error: %v", err)
			}
			if tt.wantEnv {
				requireDDCEnv(t, r, tt.opts.DDCPath)
			}
		})
	}
}

func requireDDCEnv(t *testing.T, r *runner.Runner, wantPath string) {
	t.Helper()
	want := ddc.EnvOverride(wantPath)
	for _, e := range r.Env {
		if e == want {
			return
		}
	}
	t.Errorf("runner.Env should contain %q, got %v", want, r.Env)
}

func TestSetupDDC_LocalWithPath(t *testing.T) {
	ddcDir := filepath.Join(t.TempDir(), "ddc")
	r := runner.NewRunner(false, false)
	b := NewBuilder(BuildOptions{DDCMode: "local", DDCPath: ddcDir}, r)
	err := b.setupDDC()
	if err != nil {
		t.Fatalf("setupDDC() error: %v", err)
	}

	if _, err := os.Stat(ddcDir); err != nil {
		t.Errorf("DDC directory not created: %v", err)
	}

	requireDDCEnv(t, r, ddcDir)
}

func TestSetupDDC_InvalidMode(t *testing.T) {
	r := runner.NewRunner(false, false)
	b := NewBuilder(BuildOptions{DDCMode: "invalid"}, r)
	err := b.setupDDC()
	if err == nil {
		t.Fatal("setupDDC() should error on invalid mode")
	}
	if !strings.Contains(err.Error(), "unsupported DDC mode") {
		t.Errorf("error should mention unsupported mode, got: %v", err)
	}
}

func TestSetupDDC_CreatesNestedDirectory(t *testing.T) {
	ddcDir := filepath.Join(t.TempDir(), "nested", "ddc")
	r := runner.NewRunner(false, false)
	b := NewBuilder(BuildOptions{DDCMode: "local", DDCPath: ddcDir}, r)
	err := b.setupDDC()
	if err != nil {
		t.Fatalf("setupDDC() error: %v", err)
	}

	if _, err := os.Stat(ddcDir); err != nil {
		t.Errorf("DDC directory not created (nested): %v", err)
	}
}

func TestSetupDDC_LocalMkdirAllError(t *testing.T) {
	// A file in place of an ancestor dir makes os.MkdirAll fail.
	blocker := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	r := runner.NewRunner(false, false)
	b := NewBuilder(BuildOptions{DDCMode: "local", DDCPath: filepath.Join(blocker, "ddc")}, r)
	err := b.setupDDC()
	if err == nil || !strings.Contains(err.Error(), "creating DDC directory") {
		t.Errorf("expected DDC dir creation error, got %v", err)
	}
}
