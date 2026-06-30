package mcp

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
	"github.com/jpvelasco/ludus/internal/engine"
	"github.com/jpvelasco/ludus/internal/game"
	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/toolchain"
	"github.com/jpvelasco/ludus/internal/wsl"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// builds is the package-level build manager, initialized in runMCP.
var builds *buildManager

// --- Input types ---

type engineBuildStartInput struct {
	Jobs      int    `json:"jobs,omitempty" jsonschema:"Max parallel compile jobs (0 = auto-detect from RAM)"`
	Backend   string `json:"backend,omitempty" jsonschema:"Build backend: native, docker, podman, or wsl2 (default: from config)"`
	NoCache   bool   `json:"no_cache,omitempty" jsonschema:"Disable build caching (force rebuild even if inputs are unchanged)"`
	DryRun    bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
	SkipSetup bool   `json:"skip_setup,omitempty" jsonschema:"Skip the Setup step (Setup.sh/Setup.bat) when dependencies are already fetched; avoids redist-installer hangs on headless Windows (native backend only)"`
	WSLNative bool   `json:"wsl_native,omitempty" jsonschema:"Sync engine source to WSL2 native ext4 for faster builds (requires backend=wsl2)"`
	WSLDistro string `json:"wsl_distro,omitempty" jsonschema:"WSL2 distro override (default: first running WSL2 distro)"`
}

type gameBuildStartInput struct {
	SkipCook  bool   `json:"skip_cook,omitempty" jsonschema:"Skip content cooking (use previously cooked content)"`
	Backend   string `json:"backend,omitempty" jsonschema:"Build backend: native, docker, podman, or wsl2 (default: from config)"`
	Arch      string `json:"arch,omitempty" jsonschema:"Target CPU architecture: amd64 or arm64 (default: from config)"`
	Config    string `json:"config,omitempty" jsonschema:"Build configuration: Development or Shipping (default: Development)"`
	Jobs      int    `json:"jobs,omitempty" jsonschema:"Max parallel compile actions (0 = auto-detect from RAM, halved for cross-compile)"`
	NoCache   bool   `json:"no_cache,omitempty" jsonschema:"Disable build caching (force rebuild even if inputs are unchanged)"`
	DryRun    bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
	WSLDistro string `json:"wsl_distro,omitempty" jsonschema:"WSL2 distro override (default: first running WSL2 distro)"`
}

type gameClientStartInput struct {
	Platform string `json:"platform,omitempty" jsonschema:"Target platform: Linux or Win64"`
	SkipCook bool   `json:"skip_cook,omitempty" jsonschema:"Skip content cooking"`
	Backend  string `json:"backend,omitempty" jsonschema:"Build backend: native, docker, podman, or wsl2 (default: from config)"`
	Jobs     int    `json:"jobs,omitempty" jsonschema:"Max parallel compile actions (0 = auto-detect from RAM, halved for cross-compile)"`
	NoCache  bool   `json:"no_cache,omitempty" jsonschema:"Disable build caching (force rebuild even if inputs are unchanged)"`
	DryRun   bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type buildStatusInput struct {
	BuildID string `json:"build_id,omitempty" jsonschema:"Build ID to check. Omit to list all builds."`
	Cancel  bool   `json:"cancel,omitempty" jsonschema:"Cancel the specified build (requires build_id)"`
}

// --- Result types ---

type buildStartResult struct {
	BuildID string `json:"build_id"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

type buildStatusResult struct {
	BuildID        string  `json:"build_id"`
	Type           string  `json:"type"`
	Status         string  `json:"status"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	Result         any     `json:"result,omitempty"`
	Error          string  `json:"error,omitempty"`
	OutputTail     string  `json:"output_tail,omitempty"`
	OutputBytes    int     `json:"output_bytes"`
}

type buildListResult struct {
	Builds []buildStatusResult `json:"builds"`
}

// --- Registration ---

func registerAsyncBuildTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_engine_build_start",
		Description: "Start an engine build asynchronously. Returns a build ID immediately — poll with ludus_build_status. Use this instead of ludus_engine_build to avoid blocking the agent for hours.",
	}, handleEngineBuildStart)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_game_build_start",
		Description: "Start a game server build asynchronously. Returns a build ID immediately — poll with ludus_build_status. Use this instead of ludus_game_build to avoid blocking the agent for hours.",
	}, handleGameBuildStart)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_game_client_start",
		Description: "Start a game client build asynchronously. Returns a build ID immediately — poll with ludus_build_status. Use this instead of ludus_game_client to avoid blocking the agent for hours.",
	}, handleGameClientStart)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_build_status",
		Description: "Check the status of an async build. With build_id: returns status and last 100 lines of output. With build_id + cancel=true: cancels the build. Without build_id: lists all builds.",
	}, handleBuildStatus)
}

// --- snapshotConfig ---

// snapshotConfig creates a deep copy of the config so async builds don't
// race with each other or with sync tool calls that mutate globals.Cfg.
func snapshotConfig() *config.Config {
	cfg := globals.Cfg
	snap := *cfg

	// Deep-copy sub-structs that contain reference types
	if cfg.AWS.Tags != nil {
		snap.AWS.Tags = make(map[string]string, len(cfg.AWS.Tags))
		maps.Copy(snap.AWS.Tags, cfg.AWS.Tags)
	}
	if cfg.Game.ContentValidation != nil {
		cv := *cfg.Game.ContentValidation
		if cfg.Game.ContentValidation.PluginContentDirs != nil {
			cv.PluginContentDirs = make([]string, len(cfg.Game.ContentValidation.PluginContentDirs))
			copy(cv.PluginContentDirs, cfg.Game.ContentValidation.PluginContentDirs)
		}
		snap.Game.ContentValidation = &cv
	}
	if cfg.CI.RunnerLabels != nil {
		snap.CI.RunnerLabels = make([]string, len(cfg.CI.RunnerLabels))
		copy(snap.CI.RunnerLabels, cfg.CI.RunnerLabels)
	}

	return &snap
}

// --- Handlers ---

func handleEngineBuildStart(_ context.Context, _ *mcp.CallToolRequest, input engineBuildStartInput) (*mcp.CallToolResult, any, error) {
	cfg := snapshotConfig()

	be := resolveBackend(input.Backend, cfg.Engine.Backend)
	if dockerbuild.IsContainerBackend(be) {
		return toolError("async container engine builds are not yet supported; use ludus_engine_build for container backends")
	}

	engineHash := cache.EngineKey(cfg)
	if hit := checkCacheHit(input.NoCache, cache.StageEngine, engineHash,
		engineResult{Success: true, EnginePath: cfg.Engine.SourcePath, Output: "Engine build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	jobs := input.Jobs
	if jobs == 0 {
		jobs = cfg.Engine.MaxJobs
	}
	dryRun := input.DryRun || globals.DryRun

	if dockerbuild.IsWSL2Backend(be) {
		return startWSL2EngineBuild(cfg, input, dryRun, jobs, engineHash)
	}
	return startNativeEngineBuild(cfg, dryRun, jobs, engineHash, input.SkipSetup)
}

func handleGameBuildStart(_ context.Context, _ *mcp.CallToolRequest, input gameBuildStartInput) (*mcp.CallToolResult, any, error) {
	cfg := snapshotConfig()
	applyArchOverride(cfg, input.Arch)

	be := resolveBackend(input.Backend, cfg.Engine.Backend)
	if dockerbuild.IsContainerBackend(be) {
		return toolError("async container game builds are not yet supported; use ludus_game_build for container backends")
	}

	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)
	if hit := checkCacheHit(input.NoCache, cache.StageGameServer, serverHash,
		gameBuildResult{Success: true, Output: "Game server build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	dryRun := input.DryRun || globals.DryRun

	if dockerbuild.IsWSL2Backend(be) {
		return startWSL2GameBuild(cfg, input, dryRun, serverHash)
	}
	return startNativeGameBuild(cfg, input, dryRun, serverHash)
}

func handleGameClientStart(_ context.Context, _ *mcp.CallToolRequest, input gameClientStartInput) (*mcp.CallToolResult, any, error) {
	cfg := snapshotConfig()

	platform := input.Platform
	if platform == "" {
		platform = "Linux"
	}

	be := resolveBackend(input.Backend, cfg.Engine.Backend)
	if dockerbuild.IsContainerBackend(be) {
		return toolError("async container client builds are not yet supported; use ludus_game_client for container backends")
	}
	if dockerbuild.IsWSL2Backend(be) {
		return toolError("async WSL2 client builds are not yet supported; use ludus_game_client for WSL2 backends")
	}

	engineHash := cache.EngineKey(cfg)
	clientHash := cache.GameClientKey(cfg, engineHash, platform)
	if hit := checkCacheHit(input.NoCache, cache.StageGameClient, clientHash,
		gameBuildResult{Success: true, Output: "Game client build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	return startNativeClientBuild(cfg, input, platform, input.DryRun || globals.DryRun, clientHash)
}

func handleBuildStatus(_ context.Context, _ *mcp.CallToolRequest, input buildStatusInput) (*mcp.CallToolResult, any, error) {
	if builds == nil {
		return toolError("build manager not initialized")
	}

	// Cancel mode
	if input.BuildID != "" && input.Cancel {
		if err := builds.CancelBuild(input.BuildID); err != nil {
			return toolError(err.Error())
		}
		return resultOK(map[string]any{
			"success":  true,
			"build_id": input.BuildID,
			"message":  "Build cancellation requested.",
		})
	}

	// Single build status
	if input.BuildID != "" {
		entry, ok := builds.Get(input.BuildID)
		if !ok {
			return toolError(fmt.Sprintf("build %q not found", input.BuildID))
		}
		return resultOK(buildEntryToResult(entry, true))
	}

	// List all builds
	entries := builds.List()
	list := buildListResult{
		Builds: make([]buildStatusResult, 0, len(entries)),
	}
	for _, entry := range entries {
		list.Builds = append(list.Builds, buildEntryToResult(entry, false))
	}

	return resultOK(list)
}

// buildEntryToResult converts a buildEntry to a buildStatusResult.
// When detailed is true, includes output tail and result payload.
func buildEntryToResult(entry *buildEntry, detailed bool) buildStatusResult {
	elapsed := time.Since(entry.StartedAt).Seconds()
	if entry.Status != buildStatusRunning {
		elapsed = entry.EndedAt.Sub(entry.StartedAt).Seconds()
	}

	r := buildStatusResult{
		BuildID:        entry.ID,
		Type:           string(entry.Type),
		Status:         string(entry.Status),
		ElapsedSeconds: elapsed,
		Error:          entry.Error,
		OutputBytes:    entry.Output.Len(),
	}

	if detailed {
		r.Result = entry.Result
		r.OutputTail = entry.Output.tailLines(100)
	}

	return r
}

// --- Build starters ---

func startWSL2EngineBuild(cfg *config.Config, input engineBuildStartInput, dryRun bool, jobs int, engineHash string) (*mcp.CallToolResult, any, error) {
	id, err := builds.Start(buildTypeEngineBuild, func(ctx context.Context, buf *syncBuffer) (any, error) {
		r := &runner.Runner{Stdout: buf, Stderr: buf, Verbose: true, DryRun: dryRun}

		w, wslErr := wsl.New(r, input.WSLDistro)
		if wslErr != nil {
			return nil, fmt.Errorf("WSL2 init failed: %w\n\nIf WSL2 is not available, use Podman instead: ludus_engine_build with backend=podman", wslErr)
		}

		version, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
		enginePath, ddcPath, pathErr := resolveWSL2Paths(ctx, r, w, cfg.Engine.SourcePath, version, input.WSLNative)
		if pathErr != nil {
			return nil, fmt.Errorf("WSL2 sync failed: %w", pathErr)
		}

		br, buildErr := wsl.BuildEngine(ctx, w, wsl.EngineOptions{
			SourcePath: cfg.Engine.SourcePath,
			MaxJobs:    jobs,
			WSLNative:  input.WSLNative,
			Version:    version,
		})
		if buildErr != nil {
			return nil, fmt.Errorf("WSL2 engine build failed: %w", buildErr)
		}

		result := engineResult{Success: br.Success, EnginePath: cfg.Engine.SourcePath, DurationSeconds: br.Duration}
		if result.Success {
			saveWSL2EngineResult(enginePath, ddcPath, engineHash, input.WSLNative, dryRun)
		}
		return result, nil
	})
	if err != nil {
		return toolError(err.Error())
	}
	return resultOK(buildStartResult{BuildID: id, Type: string(buildTypeEngineBuild), Message: "WSL2 engine build started. Poll with ludus_build_status."})
}

func startNativeEngineBuild(cfg *config.Config, dryRun bool, jobs int, engineHash string, skipSetup bool) (*mcp.CallToolResult, any, error) {
	id, err := builds.Start(buildTypeEngineBuild, func(ctx context.Context, buf *syncBuffer) (any, error) {
		r := &runner.Runner{Stdout: buf, Stderr: buf, Verbose: true, DryRun: dryRun}

		br, buildErr := engine.NewBuilder(engine.BuildOptions{
			SourcePath: cfg.Engine.SourcePath,
			MaxJobs:    jobs,
			Verbose:    true,
			SkipSetup:  skipSetup,
		}, r).Build(ctx)
		if buildErr != nil {
			return nil, fmt.Errorf("engine build failed: %w", buildErr)
		}

		result := engineResult{Success: br.Success, EnginePath: cfg.Engine.SourcePath, DurationSeconds: br.Duration}
		if result.Success {
			saveCache(cache.StageEngine, engineHash, dryRun)
		}
		return result, nil
	})
	if err != nil {
		return toolError(err.Error())
	}
	return resultOK(buildStartResult{BuildID: id, Type: string(buildTypeEngineBuild), Message: "Engine build started. Poll with ludus_build_status."})
}

func startWSL2GameBuild(cfg *config.Config, input gameBuildStartInput, dryRun bool, serverHash string) (*mcp.CallToolResult, any, error) {
	id, err := builds.Start(buildTypeGameBuild, func(ctx context.Context, buf *syncBuffer) (any, error) {
		r := &runner.Runner{Stdout: buf, Stderr: buf, Verbose: true, DryRun: dryRun}

		s, stErr := state.Load()
		if stErr != nil {
			return nil, fmt.Errorf("loading state: %w", stErr)
		}
		if s.WSL2Engine == nil {
			return nil, fmt.Errorf("no WSL2 engine build found; run ludus_engine_build_start with backend=wsl2 first")
		}

		w, wslErr := wsl.New(r, input.WSLDistro)
		if wslErr != nil {
			return nil, fmt.Errorf("WSL2 init failed: %w\n\nIf WSL2 is not available, use Podman instead: ludus_game_build with backend=podman", wslErr)
		}

		ddcMode, ddcPath, _, ddcErr := globals.ResolveDDC()
		if ddcErr != nil {
			return nil, fmt.Errorf("resolving DDC: %w", ddcErr)
		}

		opts := wsl.GameOptions{
			EnginePath:   s.WSL2Engine.EnginePath,
			ProjectPath:  cfg.Game.ProjectPath,
			ProjectName:  cfg.Game.ProjectName,
			ServerTarget: cfg.Game.ResolvedServerTarget(),
			Platform:     cfg.Game.Platform,
			Arch:         cfg.Game.ResolvedArch(),
			SkipCook:     input.SkipCook,
			ServerMap:    cfg.Game.ServerMap,
			DDCMode:      ddcMode,
			DDCPath:      resolveWSL2DDCPath(w, s.WSL2Engine, ddcMode, ddcPath),
			ServerConfig: input.Config,
			MaxJobs:      input.Jobs,
		}

		br, buildErr := wsl.BuildGame(ctx, w, opts)
		if buildErr != nil {
			return nil, fmt.Errorf("WSL2 game build failed: %w", buildErr)
		}

		result := gameBuildResult{Success: br.Success, OutputDir: br.OutputDir, Binary: br.ServerBinary, DurationSeconds: br.Duration}
		if result.Success {
			saveCache(cache.StageGameServer, serverHash, dryRun)
		}
		return result, nil
	})
	if err != nil {
		return toolError(err.Error())
	}
	return resultOK(buildStartResult{BuildID: id, Type: string(buildTypeGameBuild), Message: "WSL2 game build started. Poll with ludus_build_status."})
}

func startNativeGameBuild(cfg *config.Config, input gameBuildStartInput, dryRun bool, serverHash string) (*mcp.CallToolResult, any, error) {
	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
	ddcMode, ddcPath, _, err := globals.ResolveDDC()
	if err != nil {
		return toolError(fmt.Sprintf("resolving DDC config: %v", err))
	}

	id, err := builds.Start(buildTypeGameBuild, func(ctx context.Context, buf *syncBuffer) (any, error) {
		r := &runner.Runner{Stdout: buf, Stderr: buf, Verbose: true, DryRun: dryRun}

		opts := makeGameBuildOptsWithDDC(cfg, input.SkipCook, "", input.Config, input.Jobs, engineVersion, ddcMode, ddcPath)

		br, buildErr := game.NewBuilder(opts, r).Build(ctx)
		if buildErr != nil {
			return nil, fmt.Errorf("game server build failed: %w", buildErr)
		}

		result := gameBuildResult{Success: br.Success, OutputDir: br.OutputDir, Binary: br.ServerBinary, DurationSeconds: br.Duration}
		if result.Success {
			saveCache(cache.StageGameServer, serverHash, dryRun)
		}
		return result, nil
	})
	if err != nil {
		return toolError(err.Error())
	}
	return resultOK(buildStartResult{BuildID: id, Type: string(buildTypeGameBuild), Message: "Game server build started. Poll with ludus_build_status."})
}

func startNativeClientBuild(cfg *config.Config, input gameClientStartInput, platform string, dryRun bool, clientHash string) (*mcp.CallToolResult, any, error) {
	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
	ddcMode, ddcPath, _, err := globals.ResolveDDC()
	if err != nil {
		return toolError(fmt.Sprintf("resolving DDC config: %v", err))
	}

	id, err := builds.Start(buildTypeGameClient, func(ctx context.Context, buf *syncBuffer) (any, error) {
		r := &runner.Runner{Stdout: buf, Stderr: buf, Verbose: true, DryRun: dryRun}

		opts := makeGameBuildOptsWithDDC(cfg, input.SkipCook, platform, "", input.Jobs, engineVersion, ddcMode, ddcPath)

		br, buildErr := game.NewBuilder(opts, r).BuildClient(ctx)
		if buildErr != nil {
			return nil, fmt.Errorf("game client build failed: %w", buildErr)
		}

		result := gameBuildResult{Success: br.Success, OutputDir: br.OutputDir, Binary: br.ClientBinary, DurationSeconds: br.Duration}
		if result.Success {
			_ = state.UpdateClient(&state.ClientState{
				BinaryPath: result.Binary,
				OutputDir:  result.OutputDir,
				Platform:   platform,
				BuiltAt:    time.Now().UTC().Format(time.RFC3339),
			})
			saveCache(cache.StageGameClient, clientHash, dryRun)
		}
		return result, nil
	})
	if err != nil {
		return toolError(err.Error())
	}
	return resultOK(buildStartResult{BuildID: id, Type: string(buildTypeGameClient), Message: "Game client build started. Poll with ludus_build_status."})
}
