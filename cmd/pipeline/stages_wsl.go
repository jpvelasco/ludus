package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/internal/ddc"
	engBuilder "github.com/devrecon/ludus/internal/engine"
	gameBuilder "github.com/devrecon/ludus/internal/game"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/wsl"
)

// wsl2Fallback wraps a WSL2 init error with a Podman fallback recommendation.
func wsl2Fallback(err error) error {
	return fmt.Errorf("%w\n\nIf WSL2 is not available, use Podman instead:\n  ludus engine build --backend podman", err)
}

// buildEngineWSL2 compiles Unreal Engine inside a WSL2 distro.
// Mirrors the logic in cmd/engine/engine.go:runWSL2Build.
func (p *pipelineCtx) buildEngineWSL2(ctx context.Context) (*engBuilder.BuildResult, error) {
	w, err := wsl.New(p.r, p.wslDistro)
	if err != nil {
		return nil, wsl2Fallback(err)
	}
	fmt.Printf("    Using WSL2 distro: %s\n", w.Distro)

	wslEnginePath, wslDDCPath, err := p.resolveWSL2EnginePaths(ctx, w)
	if err != nil {
		return nil, err
	}

	result, err := wsl.BuildEngine(ctx, w, wsl.EngineOptions{
		SourcePath: p.cfg.Engine.SourcePath,
		MaxJobs:    p.cfg.Engine.MaxJobs,
		WSLNative:  p.wslNative,
		Version:    p.engineVersion,
	})
	if err != nil {
		return nil, err
	}

	p.saveWSL2EngineState(wslEnginePath, wslDDCPath)
	return result, nil
}

// resolveWSL2EnginePaths returns the WSL2 engine and DDC paths for the build.
// When --wsl-native is set it rsyncs the source to native ext4 first (fast I/O);
// otherwise it converts the Windows source path to a /mnt/ virtiofs path.
func (p *pipelineCtx) resolveWSL2EnginePaths(ctx context.Context, w *wsl.WSL2) (wslEnginePath, wslDDCPath string, err error) {
	if p.wslNative {
		fmt.Println("    Syncing engine source to WSL2 native ext4...")
		syncResult, syncErr := wsl.SyncEngine(ctx, p.r, w.Distro, wsl.SyncOptions{
			SourcePath: p.cfg.Engine.SourcePath,
			Version:    p.engineVersion,
		})
		if syncErr != nil {
			return "", "", syncErr
		}
		fmt.Printf("    Synced to %s in %.0fs\n", syncResult.WSLPath, syncResult.Duration.Seconds())
		return syncResult.WSLPath, syncResult.DDCPath, nil
	}
	enginePath := w.ToWSLPath(p.cfg.Engine.SourcePath)
	ddcPath := w.ToWSLPath(filepath.Join(filepath.Dir(p.cfg.Engine.SourcePath), ".ludus", "ddc"))
	return enginePath, ddcPath, nil
}

// saveWSL2EngineState persists the WSL2 engine and DDC paths to .ludus/state.json
// so that subsequent game builds can locate the engine without re-detection.
func (p *pipelineCtx) saveWSL2EngineState(wslEnginePath, wslDDCPath string) {
	syncTime := ""
	if p.wslNative {
		syncTime = time.Now().UTC().Format(time.RFC3339)
	}
	if err := state.UpdateWSL2Engine(&state.WSL2EngineState{
		EnginePath: wslEnginePath,
		IsNative:   p.wslNative,
		DDCPath:    wslDDCPath,
		SyncTime:   syncTime,
		BuiltAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("    Warning: failed to write state: %v\n", err)
	}
}

// buildGameWSL2 builds a dedicated server inside WSL2 using RunUAT.
// Mirrors the logic in cmd/game/game.go:runWSL2GameBuild.
func (p *pipelineCtx) buildGameWSL2(ctx context.Context, projectName string) (*gameBuilder.BuildResult, error) {
	// Load state written by buildEngineWSL2 to find the engine path.
	s, err := state.Load()
	if err != nil {
		return nil, fmt.Errorf("loading state: %w", err)
	}
	if s.WSL2Engine == nil {
		return nil, fmt.Errorf("no WSL2 engine build found; engine build stage must run first")
	}

	w, err := wsl.New(p.r, p.wslDistro)
	if err != nil {
		return nil, wsl2Fallback(err)
	}
	fmt.Printf("    Using WSL2 distro: %s\n", w.Distro)

	wslDDCPath := resolveWSL2GameDDCPath(s.WSL2Engine, p.ddcMode, p.ddcPath, w)

	opts := wsl.GameOptions{
		EnginePath:   s.WSL2Engine.EnginePath,
		ProjectPath:  p.cfg.Game.ProjectPath,
		ProjectName:  projectName,
		ServerTarget: p.cfg.Game.ResolvedServerTarget(),
		Platform:     p.cfg.Game.Platform,
		Arch:         p.arch,
		ServerMap:    p.cfg.Game.ServerMap,
		DDCMode:      p.ddcMode,
		DDCPath:      wslDDCPath,
	}

	return wsl.BuildGame(ctx, w, opts)
}

// resolveWSL2GameDDCPath returns the DDC path to use inside WSL2 for a game build.
// It prefers the path recorded in engine state (which reflects --wsl-native),
// falling back to the virtiofs host path when DDC is enabled but no state path exists.
func resolveWSL2GameDDCPath(engineState *state.WSL2EngineState, ddcMode, ddcPath string, w *wsl.WSL2) string {
	if engineState.DDCPath != "" {
		return engineState.DDCPath
	}
	if ddcMode == ddc.ModeLocal {
		return w.ToWSLPath(ddcPath)
	}
	return ""
}
