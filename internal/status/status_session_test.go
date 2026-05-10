package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/state"
)

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
	t.Chdir(dir)
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
