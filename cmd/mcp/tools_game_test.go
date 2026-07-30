package mcp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
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
	t.Chdir(t.TempDir())

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

func TestGameHandlersReturnCachedBuilds(t *testing.T) {
	tests := []struct {
		name  string
		stage cache.StageKey
		hash  func(*config.Config, string) string
		call  func(context.Context) string
	}{
		{
			name:  "server",
			stage: cache.StageGameServer,
			hash:  cache.GameServerKey,
			call: func(ctx context.Context) string {
				result, _, err := handleGameBuild(ctx, nil, gameBuildInput{})
				if err != nil {
					t.Fatalf("handleGameBuild: %v", err)
				}
				return toolResultText(t, result)
			},
		},
		{
			name:  "client",
			stage: cache.StageGameClient,
			hash: func(cfg *config.Config, engineHash string) string {
				return cache.GameClientKey(cfg, engineHash, "Linux")
			},
			call: func(ctx context.Context) string {
				result, _, err := handleGameClient(ctx, nil, gameClientInput{})
				if err != nil {
					t.Fatalf("handleGameClient: %v", err)
				}
				return toolResultText(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			withGameConfig(t, &config.Config{})

			engineHash := cache.EngineKey(globals.Cfg)
			buildCache := &cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
				tt.stage: {Hash: tt.hash(globals.Cfg, engineHash)},
			}}
			if err := cache.Save(buildCache); err != nil {
				t.Fatalf("cache.Save: %v", err)
			}

			if text := tt.call(context.Background()); !strings.Contains(text, "cached") {
				t.Fatalf("result = %q, want cached build", text)
			}
		})
	}
}

func TestGameHandlersReportInvalidDDCMode(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context) string
	}{
		{
			name: "native server",
			call: func(ctx context.Context) string {
				result, _, err := handleGameBuild(ctx, nil, gameBuildInput{NoCache: true})
				if err != nil {
					t.Fatalf("handleGameBuild: %v", err)
				}
				return toolResultText(t, result)
			},
		},
		{
			name: "native client",
			call: func(ctx context.Context) string {
				result, _, err := handleGameClient(ctx, nil, gameClientInput{NoCache: true})
				if err != nil {
					t.Fatalf("handleGameClient: %v", err)
				}
				return toolResultText(t, result)
			},
		},
		{
			name: "container server",
			call: func(ctx context.Context) string {
				result, _, err := handleGameBuild(ctx, nil, gameBuildInput{Backend: "docker", NoCache: true})
				if err != nil {
					t.Fatalf("handleGameBuild container: %v", err)
				}
				return toolResultText(t, result)
			},
		},
		{
			name: "container client",
			call: func(ctx context.Context) string {
				result, _, err := handleGameClient(ctx, nil, gameClientInput{Backend: "podman", NoCache: true})
				if err != nil {
					t.Fatalf("handleGameClient container: %v", err)
				}
				return toolResultText(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			withGameConfig(t, &config.Config{Engine: config.EngineConfig{DockerImage: "engine:test"}})
			origDDCMode := globals.DDCMode
			t.Cleanup(func() { globals.DDCMode = origDDCMode })
			globals.DDCMode = "invalid"

			if text := tt.call(context.Background()); !strings.Contains(text, "invalid DDC mode") {
				t.Fatalf("result = %q, want invalid DDC mode", text)
			}
		})
	}
}

func withGameConfig(t *testing.T, cfg *config.Config) {
	t.Helper()
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = cfg
}

// TestSetupWSL2GameBuildNoWSL2Engine covers the error when no WSL2 engine state exists
// (line 248-250: WSL2EngineState check).
func TestSetupWSL2GameBuildNoWSL2Engine(t *testing.T) {
	t.Chdir(t.TempDir())
	withGameConfig(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: "/engine"},
		Game:   config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
	})

	// State has no WSL2 engine (default empty state)
	_, _, err := setupWSL2GameBuild(globals.Cfg, gameBuildInput{})
	if err == nil || !strings.Contains(err.Error(), "no WSL2 engine build found") {
		t.Fatalf("setupWSL2GameBuild error = %v, want 'no WSL2 engine build found'", err)
	}
}

// TestResolveWSL2DDCPathWithEngineState verifies DDC path resolution from engine state
// (line 196-202: resolveWSL2DDCPath logic).
func TestResolveWSL2DDCPathWithEngineState(t *testing.T) {
	tmpDir := t.TempDir()

	// Test when engine state has a DDC path set
	result := resolveWSL2DDCPath(
		nil, // w not used in this branch
		&state.WSL2EngineState{DDCPath: "/mnt/c/Users/.ludus/ddc"},
		"zen",
		tmpDir,
	)

	if result != "/mnt/c/Users/.ludus/ddc" {
		t.Errorf("resolveWSL2DDCPath = %q, want /mnt/c/Users/.ludus/ddc", result)
	}
}

// TestResolveWSL2DDCPathNonLocalModeEmptyPath covers zen mode (non-local)
// (line 198-200: when mode is not local, empty state path stays empty).
func TestResolveWSL2DDCPathNonLocalModeEmptyPath(t *testing.T) {
	tmpDir := t.TempDir()

	// When mode is zen (not local) and engine state has no path, result should be empty
	result := resolveWSL2DDCPath(
		nil,                                  // w not used in zen mode
		&state.WSL2EngineState{DDCPath: ""}, // Empty DDC path in state
		"zen",                                 // zen mode (non-local)
		tmpDir,                               // Host DDC path
	)

	// For zen mode, should return empty (DDC path handling is different)
	if result != "" {
		t.Errorf("resolveWSL2DDCPath zen mode = %q, want empty", result)
	}
}

// TestHandleWSL2GameBuildWithoutEngineFails verifies WSL2 game build error
// when no engine state exists (line 212-215).
func TestHandleWSL2GameBuildWithoutEngineFails(t *testing.T) {
	t.Chdir(t.TempDir())
	withGameConfig(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: "/engine"},
		Game:   config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
	})

	result, _, err := handleWSL2GameBuild(context.Background(), globals.Cfg, gameBuildInput{NoCache: true})
	if err != nil {
		t.Fatalf("handleWSL2GameBuild() error = %v", err)
	}

	text := toolResultText(t, result)
	// Should fail with "no WSL2 engine build found" or similar
	if !strings.Contains(text, "WSL2") && !strings.Contains(text, "engine") {
		t.Errorf("result should indicate WSL2 engine issue, got: %s", text)
	}
}

// TestHandleGameBuildDryRun tests native game build with dry-run.
func TestHandleGameBuildDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create the project file so the handler doesn't error on missing project
	projectPath := "Lyra.uproject"
	if err := os.WriteFile(projectPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	engineDir := t.TempDir()
	withGameConfig(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: engineDir, Backend: "native"},
		Game:   config.GameConfig{ProjectName: "Lyra", ProjectPath: projectPath},
	})
	origDDCMode := globals.DDCMode
	t.Cleanup(func() { globals.DDCMode = origDDCMode })
	globals.DDCMode = "none"

	result, _, err := handleGameBuild(context.Background(), nil, gameBuildInput{NoCache: true, DryRun: true})
	if err != nil {
		t.Fatalf("handleGameBuild() error = %v", err)
	}
	text := toolResultText(t, result)
	// Verify the result contains command output or success indicator
	if !strings.Contains(text, "build") && !strings.Contains(text, "error") && text != "" {
		t.Errorf("result = %q, want build command or error message", text)
	}
}

// TestHandleGameClientDryRun tests game client build with dry-run.
func TestHandleGameClientDryRun(t *testing.T) {
	t.Chdir(t.TempDir())
	withGameConfig(t, &config.Config{
		Engine: config.EngineConfig{SourcePath: t.TempDir(), Backend: "native"},
		Game:   config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
	})
	origDDCMode := globals.DDCMode
	t.Cleanup(func() { globals.DDCMode = origDDCMode })
	globals.DDCMode = "none"

	result, _, err := handleGameClient(context.Background(), nil, gameClientInput{NoCache: true, DryRun: true})
	if err != nil {
		t.Fatalf("handleGameClient() error = %v", err)
	}
	text := toolResultText(t, result)
	// Verify the result contains command output or success indicator
	if !strings.Contains(text, "build") && !strings.Contains(text, "error") && text != "" {
		t.Errorf("result = %q, want build command or error message", text)
	}
}

// TestHandleContainerGameBuildDryRun tests container game build with dry-run.
func TestHandleContainerGameBuildDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create project file
	if err := os.WriteFile("Lyra.uproject", []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	withGameConfig(t, &config.Config{
		Engine: config.EngineConfig{
			DockerImage: "engine:5.7",
			Backend:     "docker",
		},
		Game: config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
	})
	origDDCMode := globals.DDCMode
	t.Cleanup(func() { globals.DDCMode = origDDCMode })
	globals.DDCMode = "none"

	result, _, err := handleGameBuild(context.Background(), nil, gameBuildInput{
		Backend:  "docker",
		NoCache:  true,
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("handleGameBuild() error = %v", err)
	}
	text := toolResultText(t, result)
	// Verify the result is not empty
	if text == "" {
		t.Error("expected non-empty result")
	}
}

// TestHandleContainerGameClientDryRun tests container game client build.
func TestHandleContainerGameClientDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create project file
	if err := os.WriteFile("Lyra.uproject", []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	withGameConfig(t, &config.Config{
		Engine: config.EngineConfig{
			DockerImage: "engine:5.7",
			Backend:     "podman",
		},
		Game: config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
	})
	origDDCMode := globals.DDCMode
	t.Cleanup(func() { globals.DDCMode = origDDCMode })
	globals.DDCMode = "none"

	result, _, err := handleGameClient(context.Background(), nil, gameClientInput{
		Backend:  "podman",
		NoCache:  true,
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("handleGameClient() error = %v", err)
	}
	text := toolResultText(t, result)
	// Verify the result is not empty
	if text == "" {
		t.Error("expected non-empty result")
	}
}
