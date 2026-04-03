package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
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
				hasEnv := false
				for _, e := range r.Env {
					if strings.HasPrefix(e, "UE-LocalDataCachePath=") {
						hasEnv = true
						break
					}
				}
				if !hasEnv {
					t.Error("runner.Env should contain UE-LocalDataCachePath")
				}
			}
		})
	}
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

	found := false
	for _, e := range r.Env {
		if strings.HasPrefix(e, "UE-LocalDataCachePath=") {
			found = true
			if !strings.Contains(e, ddcDir) && !strings.Contains(e, filepath.ToSlash(ddcDir)) {
				t.Errorf("env var should contain DDC path, got: %s", e)
			}
			break
		}
	}
	if !found {
		t.Error("runner.Env should contain UE-LocalDataCachePath after setupDDC()")
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
