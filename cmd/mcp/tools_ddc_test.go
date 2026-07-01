package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/ddc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/viper"
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

			// Always assert both fields — catches cases where empty wantMode/wantPath
			// means "field must remain unmodified", not "skip the check".
			if globals.Cfg.DDC.Mode != tt.wantMode {
				t.Errorf("DDC.Mode = %q, want %q", globals.Cfg.DDC.Mode, tt.wantMode)
			}
			if globals.Cfg.DDC.LocalPath != tt.wantPath {
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

		_, _, _, _, err := validateWarmPrereqs(config.Config{})
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
		_, _, _, _, err := validateWarmPrereqs(cfg)
		if err == nil {
			t.Error("expected error for non-container backend")
		}
	})

	// An explicit docker_image bypasses the backend check — the user has pointed
	// directly at a custom image without configuring a container backend.
	// If the && were flipped to || this bypass would break silently.
	t.Run("explicit docker_image bypasses backend check", func(t *testing.T) {
		origMode := globals.DDCMode
		origCfg := globals.Cfg
		t.Cleanup(func() {
			globals.DDCMode = origMode
			globals.Cfg = origCfg
		})
		globals.DDCMode = "local"
		globals.Cfg = &config.Config{}
		globals.Cfg.DDC.LocalPath = t.TempDir()

		cfg := config.Config{Engine: config.EngineConfig{
			Backend:     "native",
			DockerImage: "my-registry/engine:5.7",
		}}
		// Should NOT error on the backend check; may fail on engine image resolution
		// (state is empty), but that is a different error path.
		_, _, _, _, err := validateWarmPrereqs(cfg)
		if err != nil && err.Error() == "DDC warmup requires a container backend (set engine.backend to docker or podman in ludus.yaml)" {
			t.Error("explicit docker_image should bypass the backend check")
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

// TestHandleDDCStatus_ModeNone verifies that mode=none returns SizeBytes=0 and
// an empty path without calling DirSize(""), which on Linux/macOS would walk
// the current working directory and return garbage instead of zero.
func TestHandleDDCStatus_ModeNone(t *testing.T) {
	origMode := globals.DDCMode
	origCfg := globals.Cfg
	t.Cleanup(func() {
		globals.DDCMode = origMode
		globals.Cfg = origCfg
	})

	globals.DDCMode = ddc.ModeNone
	globals.Cfg = &config.Config{}

	result, _, err := handleDDCStatus(context.Background(), nil, ddcStatusInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %+v", result)
	}

	status := decodeDDCResult[ddcStatusResult](t, result)
	if status.Mode != ddc.ModeNone {
		t.Errorf("Mode = %q, want %q", status.Mode, ddc.ModeNone)
	}
	if status.Path != "" {
		t.Errorf("Path = %q, want empty", status.Path)
	}
	if status.SizeBytes != 0 {
		t.Errorf("SizeBytes = %d, want 0 for mode=none", status.SizeBytes)
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

func TestPersistDDCConfig(t *testing.T) {
	tests := []struct {
		name      string
		input     ddcConfigureInput
		validated string
		wantMode  string
		wantPath  string
	}{
		{name: "mode", input: ddcConfigureInput{Mode: "none"}, validated: "none", wantMode: "none", wantPath: "old-path"},
		{name: "path", input: ddcConfigureInput{LocalPath: "new-path"}, wantMode: "local", wantPath: "new-path"},
		{name: "both", input: ddcConfigureInput{Mode: "none", LocalPath: "new-path"}, validated: "none", wantMode: "none", wantPath: "new-path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input.LocalPath != "" {
				tt.input.LocalPath = filepath.Join(t.TempDir(), "new-ddc")
				tt.wantPath = tt.input.LocalPath
			}
			prepareDDCViperConfig(t)
			changed, err := persistDDCConfig(tt.input, tt.validated)
			if err != nil {
				t.Fatalf("persistDDCConfig() error = %v", err)
			}
			if !changed {
				t.Fatal("persistDDCConfig() changed = false, want true")
			}
			if globals.Cfg.DDC.Mode != tt.wantMode || globals.Cfg.DDC.LocalPath != tt.wantPath {
				t.Errorf("DDC config = %+v, want mode %q path %q", globals.Cfg.DDC, tt.wantMode, tt.wantPath)
			}
		})
	}
}

func TestPersistDDCConfigNoChanges(t *testing.T) {
	prepareDDCViperConfig(t)
	changed, err := persistDDCConfig(ddcConfigureInput{}, "")
	if err != nil || changed {
		t.Errorf("persistDDCConfig() = (%v, %v), want (false, nil)", changed, err)
	}
}

func TestPersistDDCConfigWriteFailureRollsBack(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("ddc.mode", "local")
	viper.Set("ddc.localPath", "old-path")
	globals.Cfg = &config.Config{DDC: config.DDCConfig{Mode: "local", LocalPath: "old-path"}}

	changed, err := persistDDCConfig(ddcConfigureInput{Mode: "none", LocalPath: "new-path"}, "none")
	if err == nil || changed {
		t.Fatalf("persistDDCConfig() = (%v, %v), want write error", changed, err)
	}
	if got := viper.GetString("ddc.mode"); got != "local" {
		t.Errorf("viper mode = %q, want rollback to local", got)
	}
	if got := viper.GetString("ddc.localPath"); got != "old-path" {
		t.Errorf("viper path = %q, want rollback to old-path", got)
	}
}

func TestHandleDDCConfigureInvalidMode(t *testing.T) {
	result, _, err := handleDDCConfigure(context.Background(), nil, ddcConfigureInput{Mode: "remote"})
	if err != nil {
		t.Fatalf("handleDDCConfigure() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("handleDDCConfigure() should return an error result")
	}
}

func TestHandleDDCConfigurePersistsChanges(t *testing.T) {
	prepareDDCViperConfig(t)
	origMode := globals.DDCMode
	t.Cleanup(func() { globals.DDCMode = origMode })
	globals.DDCMode = ""
	path := filepath.Join(t.TempDir(), "configured-ddc")

	result, _, err := handleDDCConfigure(context.Background(), nil, ddcConfigureInput{Mode: "local", LocalPath: path})
	if err != nil {
		t.Fatalf("handleDDCConfigure() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handleDDCConfigure() returned error: %+v", result)
	}
	got := decodeDDCResult[ddcConfigureResult](t, result)
	if got.Mode != ddc.ModeLocal || got.Path != path || !got.Persisted {
		t.Errorf("configure result = %+v, want local path %q persisted", got, path)
	}
}

func TestHandleDDCWarmDryRun(t *testing.T) {
	origMode, origCfg := globals.DDCMode, globals.Cfg
	t.Cleanup(func() {
		globals.DDCMode = origMode
		globals.Cfg = origCfg
	})
	globals.DDCMode = ddc.ModeLocal
	globals.Cfg = &config.Config{
		DDC:    config.DDCConfig{LocalPath: t.TempDir()},
		Engine: config.EngineConfig{Backend: "docker", DockerImage: "engine:5.7"},
		Game:   config.GameConfig{ProjectName: "Lyra"},
	}

	result, _, err := handleDDCWarm(context.Background(), nil, ddcWarmInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleDDCWarm() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handleDDCWarm() returned error: %+v", result)
	}
	got := decodeDDCResult[ddcWarmResult](t, result)
	if !got.Success || !strings.Contains(got.Message, "engine:5.7") || !strings.Contains(got.Message, "Lyra") {
		t.Errorf("warm result = %+v, want dry-run details", got)
	}
}

func prepareDDCViperConfig(t *testing.T) {
	t.Helper()
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	viper.Reset()
	t.Cleanup(viper.Reset)
	dir := t.TempDir()
	path := filepath.Join(dir, "ludus.yaml")
	if err := os.WriteFile(path, []byte("ddc:\n  mode: local\n  localPath: old-path\n"), 0644); err != nil {
		t.Fatal(err)
	}
	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	globals.Cfg = &config.Config{DDC: config.DDCConfig{Mode: "local", LocalPath: "old-path"}}
}
