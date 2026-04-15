package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/ddc"
	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/game"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/devrecon/ludus/internal/wsl"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type gameBuildInput struct {
	SkipCook  bool   `json:"skip_cook,omitempty" jsonschema:"Skip content cooking (use previously cooked content)"`
	Backend   string `json:"backend,omitempty" jsonschema:"Build backend: native, docker, podman, or wsl2 (default: from config)"`
	Arch      string `json:"arch,omitempty" jsonschema:"Target CPU architecture: amd64 or arm64 (default: from config)"`
	Config    string `json:"config,omitempty" jsonschema:"Build configuration: Development or Shipping (default: Development)"`
	Jobs      int    `json:"jobs,omitempty" jsonschema:"Max parallel compile actions (0 = auto-detect from RAM, halved for cross-compile)"`
	NoCache   bool   `json:"no_cache,omitempty" jsonschema:"Disable build caching (force rebuild even if inputs are unchanged)"`
	DryRun    bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
	WSLDistro string `json:"wsl_distro,omitempty" jsonschema:"WSL2 distro override (default: first running WSL2 distro)"`
}

type gameClientInput struct {
	Platform string `json:"platform,omitempty" jsonschema:"Target platform: Linux or Win64"`
	SkipCook bool   `json:"skip_cook,omitempty" jsonschema:"Skip content cooking"`
	Backend  string `json:"backend,omitempty" jsonschema:"Build backend: native, docker, or podman (default: from config)"`
	Jobs     int    `json:"jobs,omitempty" jsonschema:"Max parallel compile actions (0 = auto-detect from RAM, halved for cross-compile)"`
	NoCache  bool   `json:"no_cache,omitempty" jsonschema:"Disable build caching (force rebuild even if inputs are unchanged)"`
	DryRun   bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type gameBuildResult struct {
	Success         bool    `json:"success"`
	OutputDir       string  `json:"output_dir,omitempty"`
	Binary          string  `json:"binary,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	Output          string  `json:"output,omitempty"`
	Error           string  `json:"error,omitempty"`
}

func registerGameTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_game_build",
		Description: "Build the UE5 game as a Linux dedicated server via RunUAT BuildCookRun. Use backend='podman' or 'docker' to build inside a pre-built engine container image, or backend='wsl2' to build in a WSL2 Linux distro. This is a long-running operation.",
	}, handleGameBuild)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_game_client",
		Description: "Build the standalone game client for Linux or Win64 via RunUAT BuildCookRun. Use backend='podman' or 'docker' for Linux-only container builds. This is a long-running operation.",
	}, handleGameClient)
}

func makeGameBuildOpts(cfg *config.Config, skipCook bool, clientPlatform, serverConfig string, jobs int) (game.BuildOptions, error) {
	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)

	ddcMode, ddcPath, err := globals.ResolveDDC()
	if err != nil {
		return game.BuildOptions{}, fmt.Errorf("resolving DDC config: %w", err)
	}

	return game.BuildOptions{
		EnginePath:     cfg.Engine.SourcePath,
		ProjectPath:    cfg.Game.ProjectPath,
		ProjectName:    cfg.Game.ProjectName,
		ServerTarget:   cfg.Game.ResolvedServerTarget(),
		ClientTarget:   cfg.Game.ResolvedClientTarget(),
		GameTarget:     cfg.Game.ResolvedGameTarget(),
		Platform:       cfg.Game.Platform,
		Arch:           cfg.Game.ResolvedArch(),
		ClientPlatform: clientPlatform,
		SkipCook:       skipCook,
		ServerMap:      cfg.Game.ServerMap,
		EngineVersion:  engineVersion,
		ServerConfig:   serverConfig,
		MaxJobs:        jobs,
		DDCMode:        ddcMode,
		DDCPath:        ddcPath,
	}, nil
}

func handleGameBuild(ctx context.Context, _ *mcp.CallToolRequest, input gameBuildInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg.Clone()

	applyArchOverride(&cfg, input.Arch)

	be := resolveBackend(input.Backend, cfg.Engine.Backend)

	if dockerbuild.IsContainerBackend(be) {
		return handleContainerGameBuild(ctx, &cfg, input, be)
	}
	if dockerbuild.IsWSL2Backend(be) {
		return handleWSL2GameBuild(ctx, &cfg, input)
	}

	engineHash := cache.EngineKey(&cfg)
	serverHash := cache.GameServerKey(&cfg, engineHash)
	if hit := checkCacheHit(input.NoCache, cache.StageGameServer, serverHash,
		gameBuildResult{Success: true, Output: "Game server build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	opts, err := makeGameBuildOpts(&cfg, input.SkipCook, "", input.Config, input.Jobs)
	if err != nil {
		return resultErr(gameBuildResult{Error: err.Error()})
	}
	r := newToolRunner(input.DryRun)
	b := game.NewBuilder(opts, r)

	var result gameBuildResult

	captured, err := withCapture(func() error {
		br, buildErr := b.Build(ctx)
		if br != nil {
			result.Success = br.Success
			result.OutputDir = br.OutputDir
			result.Binary = br.ServerBinary
			result.DurationSeconds = br.Duration
		}
		return buildErr
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = fmt.Sprintf("game server build failed: %v", err)
		return resultErr(result)
	}

	if result.Success {
		saveCache(cache.StageGameServer, serverHash)
	}

	return resultOK(result)
}

func handleContainerGameBuild(ctx context.Context, cfg *config.Config, input gameBuildInput, be string) (*mcp.CallToolResult, any, error) {
	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)
	if hit := checkCacheHit(input.NoCache, cache.StageGameServer, serverHash,
		gameBuildResult{Success: true, Output: "Game server build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	opts, err := globals.ResolveContainerGameOptions(cfg, be)
	if err != nil {
		return resultErr(gameBuildResult{Error: err.Error()})
	}
	opts.ServerTarget = cfg.Game.ResolvedServerTarget()
	opts.GameTarget = cfg.Game.ResolvedGameTarget()
	opts.SkipCook = input.SkipCook
	opts.ServerMap = cfg.Game.ServerMap

	r := newToolRunner(input.DryRun)
	b := dockerbuild.NewDockerGameBuilder(opts, r)

	var result gameBuildResult

	captured, err := withCapture(func() error {
		br, buildErr := b.Build(ctx)
		if br != nil {
			result.Success = br.Success
			result.OutputDir = br.OutputDir
			result.Binary = br.ServerBinary
			result.DurationSeconds = br.Duration
		}
		return buildErr
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = fmt.Sprintf("container game build failed: %v", err)
		return resultErr(result)
	}

	if result.Success {
		saveCache(cache.StageGameServer, serverHash)
	}

	return resultOK(result)
}

// resolveWSL2DDCPath returns the WSL DDC path for a game build. It prefers
// the path stored in state (which respects IsNative from the engine build),
// falling back to a virtiofs-converted host path.
func resolveWSL2DDCPath(w *wsl.WSL2, engineState *state.WSL2EngineState, ddcMode, hostDDCPath string) string {
	wslDDCPath := engineState.DDCPath
	if ddcMode == ddc.ModeLocal && wslDDCPath == "" {
		wslDDCPath = w.ToWSLPath(hostDDCPath)
	}
	return wslDDCPath
}

func handleWSL2GameBuild(ctx context.Context, cfg *config.Config, input gameBuildInput) (*mcp.CallToolResult, any, error) {
	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)
	if hit := checkCacheHit(input.NoCache, cache.StageGameServer, serverHash,
		gameBuildResult{Success: true, Output: "Game server build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	s, err := state.Load()
	if err != nil {
		return resultErr(gameBuildResult{Error: fmt.Sprintf("loading state: %v", err)})
	}
	if s.WSL2Engine == nil {
		return resultErr(gameBuildResult{Error: "no WSL2 engine build found; run ludus_engine_build with backend=wsl2 first"})
	}

	r := newToolRunner(input.DryRun)
	w, err := wsl.New(r, input.WSLDistro)
	if err != nil {
		return resultErr(gameBuildResult{Error: fmt.Sprintf("WSL2 init failed: %v", err)})
	}

	ddcMode, ddcPath, err := globals.ResolveDDC()
	if err != nil {
		return resultErr(gameBuildResult{Error: fmt.Sprintf("resolving DDC: %v", err)})
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

	var result gameBuildResult

	captured, err := withCapture(func() error {
		br, buildErr := wsl.BuildGame(ctx, w, opts)
		if br != nil {
			result.Success = br.Success
			result.OutputDir = br.OutputDir
			result.Binary = br.ServerBinary
			result.DurationSeconds = br.Duration
		}
		return buildErr
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = fmt.Sprintf("WSL2 game build failed: %v", err)
		return resultErr(result)
	}

	if result.Success {
		saveCache(cache.StageGameServer, serverHash)
	}

	return resultOK(result)
}

func handleGameClient(ctx context.Context, _ *mcp.CallToolRequest, input gameClientInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg.Clone()

	platform := input.Platform
	if platform == "" {
		platform = "Linux"
	}

	be := resolveBackend(input.Backend, cfg.Engine.Backend)

	if dockerbuild.IsContainerBackend(be) {
		return handleContainerGameClient(ctx, &cfg, input, platform, be)
	}

	engineHash := cache.EngineKey(&cfg)
	clientHash := cache.GameClientKey(&cfg, engineHash, platform)
	if hit := checkCacheHit(input.NoCache, cache.StageGameClient, clientHash,
		gameBuildResult{Success: true, Output: "Game client build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	opts, err := makeGameBuildOpts(&cfg, input.SkipCook, platform, "", input.Jobs)
	if err != nil {
		return resultErr(gameBuildResult{Error: err.Error()})
	}
	r := newToolRunner(input.DryRun)
	b := game.NewBuilder(opts, r)

	var result gameBuildResult

	captured, err := withCapture(func() error {
		br, buildErr := b.BuildClient(ctx)
		if br != nil {
			result.Success = br.Success
			result.OutputDir = br.OutputDir
			result.Binary = br.ClientBinary
			result.DurationSeconds = br.Duration
		}
		return buildErr
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = fmt.Sprintf("game client build failed: %v", err)
		return resultErr(result)
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

	return resultOK(result)
}

func handleContainerGameClient(ctx context.Context, cfg *config.Config, input gameClientInput, platform string, be string) (*mcp.CallToolResult, any, error) {
	engineHash := cache.EngineKey(cfg)
	clientHash := cache.GameClientKey(cfg, engineHash, platform)
	if hit := checkCacheHit(input.NoCache, cache.StageGameClient, clientHash,
		gameBuildResult{Success: true, Output: "Game client build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	opts, err := globals.ResolveContainerGameOptions(cfg, be)
	if err != nil {
		return resultErr(gameBuildResult{Error: err.Error()})
	}
	opts.ClientTarget = cfg.Game.ResolvedClientTarget()
	opts.ClientPlatform = platform
	opts.SkipCook = input.SkipCook

	r := newToolRunner(input.DryRun)
	b := dockerbuild.NewDockerGameBuilder(opts, r)

	var result gameBuildResult

	captured, err := withCapture(func() error {
		br, buildErr := b.BuildClient(ctx)
		if br != nil {
			result.Success = br.Success
			result.OutputDir = br.OutputDir
			result.Binary = br.ClientBinary
			result.DurationSeconds = br.Duration
		}
		return buildErr
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = fmt.Sprintf("container client build failed: %v", err)
		return resultErr(result)
	}

	if result.Success {
		_ = state.UpdateClient(&state.ClientState{
			BinaryPath: result.Binary,
			OutputDir:  result.OutputDir,
			Platform:   platform,
			BuiltAt:    time.Now().UTC().Format(time.RFC3339),
		})
		saveCache(cache.StageGameClient, clientHash)
	}

	return resultOK(result)
}
