package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/ddc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestValidateConfigureMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		want    string
		wantErr bool
	}{
		{"empty is passthrough", "", "", false},
		{"local is valid", "local", "local", false},
		{"none is valid", "none", "none", false},
		{"invalid errors", "garbage", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateConfigureMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateConfigureMode(%q) error = %v, wantErr %v", tt.mode, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("validateConfigureMode(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestApplyDDCConfig(t *testing.T) {
	tests := []struct {
		name      string
		input     ddcConfigureInput
		validated string
		wantMode  string
		wantPath  string
	}{
		{"sets mode only", ddcConfigureInput{Mode: "none"}, "none", "none", ""},
		{"sets path only", ddcConfigureInput{LocalPath: "/new/path"}, "", "", "/new/path"},
		{"sets both", ddcConfigureInput{Mode: "local", LocalPath: "/ddc"}, "local", "local", "/ddc"},
		{"sets neither", ddcConfigureInput{}, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCfg := globals.Cfg
			t.Cleanup(func() { globals.Cfg = origCfg })
			globals.Cfg = &config.Config{}

			applyDDCConfig(tt.input, tt.validated)

			if tt.wantMode != "" && globals.Cfg.DDC.Mode != tt.wantMode {
				t.Errorf("DDC.Mode = %q, want %q", globals.Cfg.DDC.Mode, tt.wantMode)
			}
			if tt.wantPath != "" && globals.Cfg.DDC.LocalPath != tt.wantPath {
				t.Errorf("DDC.LocalPath = %q, want %q", globals.Cfg.DDC.LocalPath, tt.wantPath)
			}
		})
	}
}

func TestValidateWarmPrereqs(t *testing.T) {
	t.Run("mode none errors", func(t *testing.T) {
		origMode := globals.DDCMode
		origCfg := globals.Cfg
		t.Cleanup(func() {
			globals.DDCMode = origMode
			globals.Cfg = origCfg
		})
		globals.DDCMode = "none"
		globals.Cfg = &config.Config{}

		_, _, _, err := validateWarmPrereqs(config.Config{})
		if err == nil {
			t.Error("expected error for mode=none")
		}
	})

	t.Run("no container backend errors", func(t *testing.T) {
		origMode := globals.DDCMode
		origCfg := globals.Cfg
		t.Cleanup(func() {
			globals.DDCMode = origMode
			globals.Cfg = origCfg
		})
		globals.DDCMode = "local"
		globals.Cfg = &config.Config{}
		globals.Cfg.DDC.LocalPath = t.TempDir()

		cfg := config.Config{Engine: config.EngineConfig{Backend: "native"}}
		_, _, _, err := validateWarmPrereqs(cfg)
		if err == nil {
			t.Error("expected error for non-container backend")
		}
	})
}

func decodeDDCResult[T any](t *testing.T, result *mcpsdk.CallToolResult) T {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected at least 1 content item")
	}
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected *mcpsdk.TextContent, got %T", result.Content[0])
	}
	var v T
	if err := json.Unmarshal([]byte(tc.Text), &v); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	return v
}

func TestHandleDDCStatus(t *testing.T) {
	origMode := globals.DDCMode
	origCfg := globals.Cfg
	t.Cleanup(func() {
		globals.DDCMode = origMode
		globals.Cfg = origCfg
	})

	ddcDir := t.TempDir()
	// Create a small test file so DirSize returns nonzero
	if err := os.WriteFile(filepath.Join(ddcDir, "test.udd"), make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}

	globals.DDCMode = ddc.ModeLocal
	globals.Cfg = &config.Config{}
	globals.Cfg.DDC.LocalPath = ddcDir

	result, _, err := handleDDCStatus(context.Background(), nil, ddcStatusInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success result")
	}

	status := decodeDDCResult[ddcStatusResult](t, result)
	if status.Mode != ddc.ModeLocal {
		t.Errorf("Mode = %q, want %q", status.Mode, ddc.ModeLocal)
	}
	if status.Path != ddcDir {
		t.Errorf("Path = %q, want %q", status.Path, ddcDir)
	}
	if status.SizeBytes != 1024 {
		t.Errorf("SizeBytes = %d, want 1024", status.SizeBytes)
	}
}

func TestHandleDDCClean(t *testing.T) {
	origMode := globals.DDCMode
	origCfg := globals.Cfg
	t.Cleanup(func() {
		globals.DDCMode = origMode
		globals.Cfg = origCfg
	})

	ddcDir := t.TempDir()
	// Create test files to clean
	for _, name := range []string{"a.udd", "b.udd"} {
		if err := os.WriteFile(filepath.Join(ddcDir, name), make([]byte, 512), 0644); err != nil {
			t.Fatal(err)
		}
	}

	globals.DDCMode = ddc.ModeLocal
	globals.Cfg = &config.Config{}
	globals.Cfg.DDC.LocalPath = ddcDir

	result, _, err := handleDDCClean(context.Background(), nil, ddcCleanInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success result")
	}

	clean := decodeDDCResult[ddcCleanResult](t, result)
	if !clean.Success {
		t.Error("expected Success = true")
	}
	if clean.BytesFreed != 1024 {
		t.Errorf("BytesFreed = %d, want 1024", clean.BytesFreed)
	}

	// Verify directory is empty
	entries, _ := os.ReadDir(ddcDir)
	if len(entries) != 0 {
		t.Errorf("DDC dir should be empty, has %d entries", len(entries))
	}
}
