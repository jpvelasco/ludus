package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/engine"
	"github.com/devrecon/ludus/internal/game"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// builds is the package-level build manager, initialized in runMCP.
var builds *buildManager

// --- Input types ---

type engineBuildStartInput struct {
	Jobs    int    `json:"jobs,omitempty" jsonschema:"Max parallel compile jobs (0 = auto-detect from RAM)"`
	Backend string `json:"backend,omitempty" jsonschema:"Build backend: native or docker (default: from config)"`
	NoCache bool   `json:"no_cache,omitempty" jsonschema:"Disable build caching (force rebuild even if inputs are unchanged)"`
	DryRun  bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type gameBuildStartInput struct {
	SkipCook bool   `json:"skip_cook,omitempty" jsonschema:"Skip content cooking (use previously cooked content)"`
	Backend  string `json:"backend,omitempty" jsonschema:"Build backend: native or docker (default: from config)"`
	Arch     string `json:"arch,omitempty" jsonschema:"Target CPU architecture: amd64 or arm64 (default: from config)"`
	Config   string `json:"config,omitempty" jsonschema:"Build configuration: Development or Shipping (default: Development)"`
	Jobs     int    `json:"jobs,omitempty" jsonschema:"Max parallel compile actions (0 = auto-detect from RAM, halved for cross-compile)"`
	NoCache  bool   `json:"no_cache,omitempty" jsonschema:"Disable build caching (force rebuild even if inputs are unchanged)"`
	DryRun   bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type gameClientStartInput struct {
	Platform string `json:"platform,omitempty" jsonschema:"Target platform: Linux or Win64"`
	SkipCook bool   `json:"skip_cook,omitempty" jsonschema:"Skip content cooking"`
	Backend  string `json:"backend,omitempty" jsonschema:"Build backend: native or docker (default: from config)"`
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
		for k, v := range cfg.AWS.Tags {
			snap.AWS.Tags[k] = v
		}
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

	// Only native backend supported for async (docker engine build uses withCapture differently)
	be := resolveBackend(input.Backend, cfg.Engine.Backend)
	if be == "docker" {
		return toolError("async docker engine builds are not yet supported; use ludus_engine_build for docker backend")
	}

	// Check cache before launching
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

	id, err := builds.Start(buildTypeEngineBuild, func(ctx context.Context, buf *syncBuffer) (any, error) {
		r := &runner.Runner{
			Stdout:  buf,
			Stderr:  buf,
			Verbose: true,
			DryRun:  dryRun,
		}

		b := engine.NewBuilder(engine.BuildOptions{
			SourcePath: cfg.Engine.SourcePath,
			MaxJobs:    jobs,
			Verbose:    true,
		}, r)

		br, buildErr := b.Build(ctx)
		if buildErr != nil {
			return nil, fmt.Errorf("engine build failed: %w", buildErr)
		}

		result := engineResult{
			Success:         br.Success,
			EnginePath:      cfg.Engine.SourcePath,
			DurationSeconds: br.Duration,
		}

		if result.Success {
			saveCache(cache.StageEngine, engineHash)
		}

		return result, nil
	})
	if err != nil {
		return toolError(err.Error())
	}

	return resultOK(buildStartResult{
		BuildID: id,
		Type:    string(buildTypeEngineBuild),
		Message: "Engine build started. Poll with ludus_build_status.",
	})
}

func handleGameBuildStart(_ context.Context, _ *mcp.CallToolRequest, input gameBuildStartInput) (*mcp.CallToolResult, any, error) {
	cfg := snapshotConfig()

	applyArchOverride(cfg, input.Arch)

	be := resolveBackend(input.Backend, cfg.Engine.Backend)
	if be == "docker" {
		return toolError("async docker game builds are not yet supported; use ludus_game_build for docker backend")
	}

	// Check cache before launching
	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)
	if hit := checkCacheHit(input.NoCache, cache.StageGameServer, serverHash,
		gameBuildResult{Success: true, Output: "Game server build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	dryRun := input.DryRun || globals.DryRun

	id, err := builds.Start(buildTypeGameBuild, func(ctx context.Context, buf *syncBuffer) (any, error) {
		r := &runner.Runner{
			Stdout:  buf,
			Stderr:  buf,
			Verbose: true,
			DryRun:  dryRun,
		}

		opts := makeGameBuildOpts(cfg, input.SkipCook, "", input.Config, input.Jobs)
		b := game.NewBuilder(opts, r)

		br, buildErr := b.Build(ctx)
		if buildErr != nil {
			return nil, fmt.Errorf("game server build failed: %w", buildErr)
		}

		result := gameBuildResult{
			Success:         br.Success,
			OutputDir:       br.OutputDir,
			Binary:          br.ServerBinary,
			DurationSeconds: br.Duration,
		}

		if result.Success {
			saveCache(cache.StageGameServer, serverHash)
		}

		return result, nil
	})
	if err != nil {
		return toolError(err.Error())
	}

	return resultOK(buildStartResult{
		BuildID: id,
		Type:    string(buildTypeGameBuild),
		Message: "Game server build started. Poll with ludus_build_status.",
	})
}

func handleGameClientStart(_ context.Context, _ *mcp.CallToolRequest, input gameClientStartInput) (*mcp.CallToolResult, any, error) {
	cfg := snapshotConfig()

	platform := input.Platform
	if platform == "" {
		platform = "Linux"
	}

	be := resolveBackend(input.Backend, cfg.Engine.Backend)
	if be == "docker" {
		return toolError("async docker client builds are not yet supported; use ludus_game_client for docker backend")
	}

	// Check cache before launching
	engineHash := cache.EngineKey(cfg)
	clientHash := cache.GameClientKey(cfg, engineHash, platform)
	if hit := checkCacheHit(input.NoCache, cache.StageGameClient, clientHash,
		gameBuildResult{Success: true, Output: "Game client build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	dryRun := input.DryRun || globals.DryRun

	id, err := builds.Start(buildTypeGameClient, func(ctx context.Context, buf *syncBuffer) (any, error) {
		r := &runner.Runner{
			Stdout:  buf,
			Stderr:  buf,
			Verbose: true,
			DryRun:  dryRun,
		}

		opts := makeGameBuildOpts(cfg, input.SkipCook, platform, "", input.Jobs)
		b := game.NewBuilder(opts, r)

		br, buildErr := b.BuildClient(ctx)
		if buildErr != nil {
			return nil, fmt.Errorf("game client build failed: %w", buildErr)
		}

		result := gameBuildResult{
			Success:         br.Success,
			OutputDir:       br.OutputDir,
			Binary:          br.ClientBinary,
			DurationSeconds: br.Duration,
		}

		// Persist client build info to state
		if result.Success {
			_ = state.UpdateClient(&state.ClientState{
				BinaryPath: result.Binary,
				OutputDir:  result.OutputDir,
				Platform:   platform,
				BuiltAt:    time.Now().UTC().Format(time.RFC3339),
			})
			saveCache(cache.StageGameClient, clientHash)
		}

		return result, nil
	})
	if err != nil {
		return toolError(err.Error())
	}

	return resultOK(buildStartResult{
		BuildID: id,
		Type:    string(buildTypeGameClient),
		Message: "Game client build started. Poll with ludus_build_status.",
	})
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
