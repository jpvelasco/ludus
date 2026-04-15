package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
)

func TestGameBuildInputWSL2Fields(t *testing.T) {
	input := gameBuildInput{
		Backend:   "wsl2",
		WSLDistro: "Debian",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded gameBuildInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Backend != "wsl2" {
		t.Errorf("Backend = %q, want %q", decoded.Backend, "wsl2")
	}
	if decoded.WSLDistro != "Debian" {
		t.Errorf("WSLDistro = %q, want %q", decoded.WSLDistro, "Debian")
	}
}

func TestGameBuildInputWSL2FieldsOmitEmpty(t *testing.T) {
	input := gameBuildInput{Backend: "native"}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	s := string(data)
	if strings.Contains(s, "wsl_distro") {
		t.Errorf("wsl_distro should be omitted when empty, got: %s", s)
	}
}

// TestGameBuildWSL2Dispatch verifies that backend=wsl2 dispatches to the
// WSL2 handler. On non-Windows / no-WSL2 CI, the handler returns a
// WSL2-specific error — proving the dispatch took the right branch.
func TestGameBuildWSL2Dispatch(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{
		Engine: config.EngineConfig{SourcePath: "/nonexistent/engine"},
		Game:   config.GameConfig{ProjectName: "TestGame"},
	}

	result, _, err := handleGameBuild(context.Background(), nil, gameBuildInput{
		Backend: "wsl2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// The WSL2 handler either fails at state.Load (no WSL2 engine state)
	// or at wsl.New() (no WSL2 available). Either way, the error message
	// should reference WSL2, proving the dispatch reached the right branch.
	assertResultContains(t, result, "WSL2")
}
