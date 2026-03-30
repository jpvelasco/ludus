package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/state"
)

func TestCheckEngineSource_Empty(t *testing.T) {
	s := CheckEngineSource("")
	if s.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", s.Status)
	}
	if s.Detail != "not configured" {
		t.Errorf("expected detail 'not configured', got %q", s.Detail)
	}
}

func TestCheckEngineSource_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	s := CheckEngineSource(tmpDir)
	if s.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", s.Status)
	}

	expectedFile := "Setup.sh"
	if runtime.GOOS == "windows" {
		expectedFile = "Setup.bat"
	}
	expectedDetail := expectedFile + " not found"
	if s.Detail != expectedDetail {
		t.Errorf("expected detail %q, got %q", expectedDetail, s.Detail)
	}
}

func TestCheckEngineSource_Exists(t *testing.T) {
	tmpDir := t.TempDir()

	setupFile := "Setup.sh"
	if runtime.GOOS == "windows" {
		setupFile = "Setup.bat"
	}

	setupPath := filepath.Join(tmpDir, setupFile)
	if err := os.WriteFile(setupPath, []byte("#!/bin/bash\necho setup"), 0644); err != nil {
		t.Fatal(err)
	}

	s := CheckEngineSource(tmpDir)
	if s.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", s.Status)
	}
	if s.Detail != tmpDir {
		t.Errorf("expected detail %q, got %q", tmpDir, s.Detail)
	}
}

func TestCheckEngineBuild_Empty(t *testing.T) {
	s := CheckEngineBuild("")
	if s.Status != "unknown" {
		t.Errorf("expected status 'unknown', got %q", s.Status)
	}
	if s.Detail != "source path not configured" {
		t.Errorf("expected detail 'source path not configured', got %q", s.Detail)
	}
}

func TestCheckEngineBuild_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	s := CheckEngineBuild(tmpDir)
	if s.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", s.Status)
	}

	expectedBinary := "UnrealEditor.exe"
	if runtime.GOOS != "windows" {
		expectedBinary = "UnrealEditor"
	}
	expectedDetail := expectedBinary + " not found"
	if s.Detail != expectedDetail {
		t.Errorf("expected detail %q, got %q", expectedDetail, s.Detail)
	}
}

func TestCheckEngineBuild_Exists(t *testing.T) {
	tmpDir := t.TempDir()

	var editorPath string
	if runtime.GOOS == "windows" {
		editorPath = filepath.Join(tmpDir, "Engine", "Binaries", "Win64", "UnrealEditor.exe")
	} else {
		editorPath = filepath.Join(tmpDir, "Engine", "Binaries", "Linux", "UnrealEditor")
	}

	if err := os.MkdirAll(filepath.Dir(editorPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(editorPath, []byte("fake editor binary"), 0644); err != nil {
		t.Fatal(err)
	}

	s := CheckEngineBuild(tmpDir)
	if s.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", s.Status)
	}

	expectedBinary := "UnrealEditor.exe"
	if runtime.GOOS != "windows" {
		expectedBinary = "UnrealEditor"
	}
	expectedDetail := expectedBinary + " found"
	if s.Detail != expectedDetail {
		t.Errorf("expected detail %q, got %q", expectedDetail, s.Detail)
	}
}

func TestCheckServerBuild_Empty(t *testing.T) {
	s := CheckServerBuild("TestGame", "", "amd64")
	if s.Status != "unknown" {
		t.Errorf("expected status 'unknown', got %q", s.Status)
	}
	if s.Detail != "output directory unknown" {
		t.Errorf("expected detail 'output directory unknown', got %q", s.Detail)
	}
}

func TestCheckServerBuild_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	s := CheckServerBuild("TestGame", tmpDir, "amd64")
	if s.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", s.Status)
	}
	if s.Detail != "not built" {
		t.Errorf("expected detail 'not built', got %q", s.Detail)
	}
}

func TestCheckServerBuild_Exists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create LinuxServer directory for amd64 arch
	serverDir := filepath.Join(tmpDir, config.ServerPlatformDir("amd64"))
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		t.Fatal(err)
	}

	s := CheckServerBuild("TestGame", tmpDir, "amd64")
	if s.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", s.Status)
	}
	if s.Detail != serverDir {
		t.Errorf("expected detail %q, got %q", serverDir, s.Detail)
	}
}

// writeState writes a state.State to .ludus/state.json in the given directory.
func writeState(t *testing.T, dir string, s *state.State) {
	t.Helper()
	stateDir := filepath.Join(dir, ".ludus")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// chdirTemp changes to a temp directory and restores on cleanup.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return dir
}

func TestCheckClientBuild_NoState(t *testing.T) {
	dir := chdirTemp(t)
	writeState(t, dir, &state.State{})

	s := CheckClientBuild("TestGame")
	if s.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", s.Status)
	}
	if s.Detail != "not built" {
		t.Errorf("expected detail 'not built', got %q", s.Detail)
	}
}

func TestCheckClientBuild_BinaryMissing(t *testing.T) {
	dir := chdirTemp(t)
	writeState(t, dir, &state.State{
		Client: &state.ClientState{
			BinaryPath: filepath.Join(dir, "nonexistent", "game.exe"),
			OutputDir:  filepath.Join(dir, "out"),
		},
	})

	s := CheckClientBuild("TestGame")
	if s.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", s.Status)
	}
}

func TestCheckClientBuild_OK(t *testing.T) {
	dir := chdirTemp(t)
	binPath := filepath.Join(dir, "out", "game.exe")
	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binPath, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	writeState(t, dir, &state.State{
		Client: &state.ClientState{
			BinaryPath: binPath,
			OutputDir:  filepath.Join(dir, "out"),
		},
	})

	s := CheckClientBuild("TestGame")
	if s.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", s.Status)
	}
}

func TestCheckGameSession_NoState(t *testing.T) {
	dir := chdirTemp(t)
	writeState(t, dir, &state.State{})

	cfg := &config.Config{Deploy: config.DeployConfig{Target: "gamelift"}}
	s := CheckGameSession(cfg)
	if s.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", s.Status)
	}
	if s.Detail != "no session" {
		t.Errorf("expected detail 'no session', got %q", s.Detail)
	}
}

func TestCheckGameSession_OK(t *testing.T) {
	dir := chdirTemp(t)
	writeState(t, dir, &state.State{
		Session: &state.SessionState{
			SessionID: "gsess-123",
			IPAddress: "1.2.3.4",
			Port:      7777,
		},
	})

	cfg := &config.Config{Deploy: config.DeployConfig{Target: "gamelift"}}
	s := CheckGameSession(cfg)
	if s.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", s.Status)
	}
}

func TestCheckGameSession_BinaryTarget(t *testing.T) {
	cfg := &config.Config{Deploy: config.DeployConfig{Target: "binary"}}
	s := CheckGameSession(cfg)
	if s.Status != "unknown" {
		t.Errorf("expected status 'unknown', got %q", s.Status)
	}
}
