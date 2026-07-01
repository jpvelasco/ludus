package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
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
