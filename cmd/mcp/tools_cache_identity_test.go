package mcp

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
)

// seedMCPOutput creates the packaged output directory for cfg so a cache hit
// passes its output validation (#739).
func seedMCPServerOutput(t *testing.T, cfg *config.Config) {
	t.Helper()
	dir := config.ResolveServerBuildDir(cfg)
	if dir == "" {
		t.Fatal("server build dir does not resolve; fix the test config")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestHandleGameBuildConfigChangesCacheKey pins #696 at the MCP entry point:
// a Shipping request must not reuse a Development-warmed entry, while an
// explicit Development request must.
func TestHandleGameBuildConfigChangesCacheKey(t *testing.T) {
	tests := []struct {
		name    string
		request string // config input of the request
		seed    string // config input the cache was warmed with
		wantHit bool
	}{
		{name: "shipping request misses development cache", request: "Shipping", seed: ""},
		{name: "explicit development reuses default cache", request: "Development", seed: "", wantHit: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			cfg := &config.Config{
				Engine: config.EngineConfig{Backend: "native", SourcePath: t.TempDir()},
				Game:   config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
			}
			withGameConfig(t, cfg)
			globals.SetGlobals(t, cfg, globals.WithDryRun(true))
			seedMCPServerOutput(t, cfg)

			engineHash := cache.EngineKey(cfg)
			if err := cache.Save(&cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
				cache.StageGameServer: {Hash: cache.GameServerKey(cfg, engineHash, tt.seed)},
			}}); err != nil {
				t.Fatalf("cache.Save: %v", err)
			}

			result, _, err := handleGameBuild(context.Background(), nil, gameBuildInput{Config: tt.request, DryRun: true})
			if err != nil {
				t.Fatalf("handleGameBuild: %v", err)
			}
			text := toolResultText(t, result)
			if got := strings.Contains(text, "cached"); got != tt.wantHit {
				t.Errorf("cache hit = %v, want %v (result: %s)", got, tt.wantHit, text)
			}
		})
	}
}

// TestHandleGameBuildBackendOverrideChangesCacheKey pins #728 for the sync
// MCP game handler: the backend input must enter the cache key, so a request
// with a different backend cannot reuse an entry warmed under the config
// backend.
func TestHandleGameBuildBackendOverrideChangesCacheKey(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{
		Engine: config.EngineConfig{Backend: "native", SourcePath: t.TempDir()},
		Game:   config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
	}
	withGameConfig(t, cfg)
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	seedMCPServerOutput(t, cfg)

	// Warm under the config backend (native), then request wsl2 via input.
	engineHash := cache.EngineKey(cfg)
	if err := cache.Save(&cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
		cache.StageGameServer: {Hash: cache.GameServerKey(cfg, engineHash, "")},
	}}); err != nil {
		t.Fatalf("cache.Save: %v", err)
	}

	result, _, err := handleGameBuild(context.Background(), nil, gameBuildInput{Backend: "wsl2", DryRun: true})
	if err != nil {
		t.Fatalf("handleGameBuild: %v", err)
	}
	if text := toolResultText(t, result); strings.Contains(text, "cached") {
		t.Errorf("wsl2 request reused the native-warmed cache entry: %s", text)
	}
}

// TestHandleEngineBuildBackendOverrideChangesCacheKey pins #728 for the sync
// MCP engine handler.
func TestHandleEngineBuildBackendOverrideChangesCacheKey(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{
		Engine: config.EngineConfig{Backend: "native", SourcePath: t.TempDir()},
		Game:   config.GameConfig{ProjectName: "Lyra"},
	}
	withEngineTestConfig(t, cfg)

	// Warm under the config backend (native), then request wsl2 via input.
	if err := cache.Save(&cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
		cache.StageEngine: {Hash: cache.EngineKey(cfg)},
	}}); err != nil {
		t.Fatalf("cache.Save: %v", err)
	}

	result, _, err := handleEngineBuild(context.Background(), nil, engineBuildInput{Backend: "wsl2"})
	if err != nil {
		t.Fatalf("handleEngineBuild: %v", err)
	}
	if text := toolResultText(t, result); strings.Contains(text, "cached") {
		t.Errorf("wsl2 request reused the native-warmed engine entry: %s", text)
	}
}

// TestHandleGameBuildStartBackendOverrideChangesCacheKey pins #728 for the
// async MCP game handler.
func TestHandleGameBuildStartBackendOverrideChangesCacheKey(t *testing.T) {
	t.Chdir(t.TempDir())
	withBuildManager(t)
	cfg := &config.Config{
		Engine: config.EngineConfig{Backend: "native", SourcePath: t.TempDir()},
		Game:   config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	seedMCPServerOutput(t, cfg)

	engineHash := cache.EngineKey(cfg)
	if err := cache.Save(&cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
		cache.StageGameServer: {Hash: cache.GameServerKey(cfg, engineHash, "")},
	}}); err != nil {
		t.Fatalf("cache.Save: %v", err)
	}

	result, _, err := handleGameBuildStart(context.Background(), nil, gameBuildStartInput{Backend: "wsl2"})
	if err != nil {
		t.Fatalf("handleGameBuildStart: %v", err)
	}
	if text := toolResultText(t, result); strings.Contains(text, "cached") {
		t.Errorf("wsl2 request reused the native-warmed cache entry: %s", text)
	}
}

// TestHandleGameBuildCacheHitRequiresServerOutput pins #739 for the MCP game
// handler: a warm entry whose packaged output is gone must be discarded.
func TestHandleGameBuildCacheHitRequiresServerOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{
		Engine: config.EngineConfig{Backend: "native", SourcePath: t.TempDir()},
		Game:   config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
	}
	withGameConfig(t, cfg)
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	// No packaged output on disk: the hit must be discarded (#739).

	engineHash := cache.EngineKey(cfg)
	if err := cache.Save(&cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
		cache.StageGameServer: {Hash: cache.GameServerKey(cfg, engineHash, "")},
	}}); err != nil {
		t.Fatalf("cache.Save: %v", err)
	}

	result, _, err := handleGameBuild(context.Background(), nil, gameBuildInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleGameBuild: %v", err)
	}
	if text := toolResultText(t, result); strings.Contains(text, "cached") {
		t.Errorf("cache hit honored with missing packaged output: %s", text)
	}
}

// TestCheckCacheHitOutputValidation pins the MCP skip seam for #739: the
// helper honors a hit only when the supplied output validator passes.
func TestCheckCacheHitOutputValidation(t *testing.T) {
	t.Chdir(t.TempDir())
	const hash = "game-hash"
	if err := cache.Save(&cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
		cache.StageGameServer: {Hash: hash},
	}}); err != nil {
		t.Fatalf("cache.Save: %v", err)
	}
	cached := gameBuildResult{Success: true, Output: "cached"}

	tests := []struct {
		name   string
		output cache.OutputExists
		want   bool
	}{
		{name: "passing validator honors hit", output: func() bool { return true }, want: true},
		{name: "failing validator discards hit", output: func() bool { return false }},
		{name: "no validator keeps current behavior", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkCacheHit(false, cache.StageGameServer, hash, tt.output, cached)
			if (got != nil) != tt.want {
				t.Errorf("checkCacheHit() hit = %v, want %v", got != nil, tt.want)
			}
		})
	}
}

// TestCheckContainerCacheHitOutputValidation pins the container MCP skip seam:
// both the packaged output validator and the engine image probe must pass.
func TestCheckContainerCacheHitOutputValidation(t *testing.T) {
	t.Chdir(t.TempDir())
	const hash = "container-hash"
	if err := cache.Save(&cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
		cache.StageGameServer: {Hash: hash},
	}}); err != nil {
		t.Fatalf("cache.Save: %v", err)
	}
	cached := gameBuildResult{Success: true, Output: "cached"}
	pass := func() bool { return true }
	fail := func() bool { return false }

	tests := []struct {
		name   string
		output cache.OutputExists
		image  cache.OutputExists
		want   bool
	}{
		{name: "both pass honors hit", output: pass, image: pass, want: true},
		{name: "missing output discards hit", output: fail, image: pass},
		{name: "missing image discards hit", output: pass, image: fail},
		{name: "no validators keep current behavior", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkContainerCacheHit(false, cache.StageGameServer, hash, tt.output, tt.image, cached)
			if (got != nil) != tt.want {
				t.Errorf("checkContainerCacheHit() hit = %v, want %v", got != nil, tt.want)
			}
		})
	}
}

// TestEngineImageProbe wraps dockerbuild.ImageExists behind the OutputExists
// seam, so the container cache check can probe images without a live daemon
// in tests.
func TestEngineImageProbe(t *testing.T) {
	r := newToolRunner(true) // dry-run: the probe echoes and succeeds
	probe := engineImageProbe(r, t.Context(), "docker", "engine:5.7")
	if !probe() {
		t.Error("engineImageProbe() = false under dry-run, want the echoed probe to succeed")
	}

}
