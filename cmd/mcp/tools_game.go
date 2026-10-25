package mcp

import (
	"context"
	"fmt"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
	"github.com/jpvelasco/ludus/internal/game"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/toolchain"
	"github.com/jpvelasco/ludus/internal/wsl"
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

// applyServerBuildResult copies a server build's outcome into the MCP result.
// Builders return a non-nil result even on failure, so the fields stay in sync
// with the error the caller reports.
func applyServerBuildResult(br *game.BuildResult, result *gameBuildResult) {
	if br == nil {
		return
	}
	result.Success = br.Success
	result.OutputDir = br.OutputDir
	result.Binary = br.ServerBinary
	result.DurationSeconds = br.Duration
}

// applyClientBuildResult copies a client build's outcome into the MCP result.
func applyClientBuildResult(br *game.ClientBuildResult, result *gameBuildResult) {
	if br == nil {
		return
	}
	result.Success = br.Success
	result.OutputDir = br.OutputDir
	result.Binary = br.ClientBinary
	result.DurationSeconds = br.Duration
}

// persistClientBuild records a successful client build: the state entry (a
// dry-run preview must not overwrite the previously saved client, #697) then
// the cache entry.
func persistClientBuild(effectiveDryRun bool, platform, clientHash string, result *gameBuildResult) {
	if !result.Success {
		return
	}
	if err := saveClientState(effectiveDryRun, platform, result); err != nil {
		result.Output += persistFailedWarn(err)
	}
	saveCache(cache.StageGameClient, clientHash, effectiveDryRun)
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

	ddcMode, ddcPath, _, err := globals.ResolveDDC()
	if err != nil {
		return game.BuildOptions{}, fmt.Errorf("resolving DDC config: %w", err)
	}

	return makeGameBuildOptsWithDDC(cfg, skipCook, clientPlatform, serverConfig, jobs, engineVersion, ddcMode, ddcPath), nil
}

func makeGameBuildOptsWithDDC(cfg *config.Config, skipCook bool, clientPlatform, serverConfig string, jobs int, engineVersion, ddcMode, ddcPath string) game.BuildOptions {
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
	}
}

func handleGameBuild(ctx context.Context, _ *mcp.CallToolRequest, input gameBuildInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg.Clone()

	applyArchOverride(&cfg, input.Arch)
	// Merge the tool input into config before hashing so cache keys and
	// build behavior share one source of truth: backend (#728) and
	// skip-cook (#558).
	globals.ApplyEngineOverrides(&cfg, input.Backend, "")
	if input.SkipCook {
		cfg.Game.SkipCook = true
	}

	be := resolveBackend(input.Backend, cfg.Engine.Backend)

	if dockerbuild.IsContainerBackend(be) {
		return handleContainerGameBuild(ctx, &cfg, input, be)
	}
	if dockerbuild.IsWSL2Backend(be) {
		return handleWSL2GameBuild(ctx, &cfg, input)
	}

	engineHash := cache.EngineKey(&cfg)
	serverHash := cache.GameServerKey(&cfg, engineHash, input.Config)
	if hit := checkCacheHit(input.NoCache, cache.StageGameServer, serverHash, globals.ServerBuildOutput(&cfg),
		gameBuildResult{Success: true, Output: "Game server build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	opts, err := makeGameBuildOpts(&cfg, cfg.Game.SkipCook, "", config.ResolvedServerBuildConfig(input.Config), input.Jobs)
	if err != nil {
		return resultErr(gameBuildResult{Error: err.Error()})
	}
	out := newToolOutput()
	r := newCaptureRunner(out, deployToolDryRun(input.DryRun))
	b := game.NewBuilder(opts, r)

	var result gameBuildResult

	br, err := b.Build(ctx)
	applyServerBuildResult(br, &result)
	result.Output = out.String()

	if err != nil {
		result.Error = fmt.Sprintf("game server build failed: %v", err)
		return resultErr(result)
	}

	if result.Success {
		saveCache(cache.StageGameServer, serverHash, deployToolDryRun(input.DryRun))
	}

	return resultOK(result)
}

func handleContainerGameBuild(ctx context.Context, cfg *config.Config, input gameBuildInput, be string) (*mcp.CallToolResult, any, error) {
	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash, input.Config)

	out := newToolOutput()
	r := newCaptureRunner(out, deployToolDryRun(input.DryRun))
	// The shared server assembly propagates the requested build configuration
	// into the generated BuildCookRun arguments (#730). The resolved value is
	// the same one hashed into the key (#696).
	opts, err := globals.ResolveContainerServerBuildOptions(cfg, be, config.ResolvedServerBuildConfig(input.Config))
	if err != nil {
		return resultErr(gameBuildResult{Error: err.Error()})
	}

	if hit := checkContainerCacheHit(input.NoCache, cache.StageGameServer, serverHash,
		globals.ServerBuildOutput(cfg), engineImageProbe(r, ctx, be, opts.EngineImage),
		gameBuildResult{Success: true, Output: "Game server build is up to date (cached), skipping."}, out); hit != nil {
		return hit, nil, nil
	}

	b := dockerbuild.NewDockerGameBuilder(opts, r)

	var result gameBuildResult

	br, err := b.Build(ctx)
	applyServerBuildResult(br, &result)
	result.Output = out.String()

	if err != nil {
		result.Error = fmt.Sprintf("container game build failed: %v", err)
		return resultErr(result)
	}

	if result.Success {
		saveCache(cache.StageGameServer, serverHash, deployToolDryRun(input.DryRun))
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

// wslGameOpts assembles the WSL2 game build options shared by the sync
// (ludus_game_build) and async (ludus_game_build_start) handlers. OutputDir is
// the PackagedServer archive root: UAT writes the platform subdirectory under
// it, and BuildGame returns that subdirectory (#751).
func wslGameOpts(cfg *config.Config, engineState *state.WSL2EngineState, w *wsl.WSL2, ddcMode, ddcPath, serverConfig string, jobs int) wsl.GameOptions {
	return wsl.GameOptions{
		EnginePath:   engineState.EnginePath,
		ProjectPath:  cfg.Game.ProjectPath,
		ProjectName:  cfg.Game.ProjectName,
		ServerTarget: cfg.Game.ResolvedServerTarget(),
		Platform:     cfg.Game.Platform,
		Arch:         cfg.Game.ResolvedArch(),
		SkipCook:     cfg.Game.SkipCook,
		ServerMap:    cfg.Game.ServerMap,
		OutputDir:    config.ResolveServerArchiveRoot(cfg),
		DDCMode:      ddcMode,
		DDCPath:      resolveWSL2DDCPath(w, engineState, ddcMode, ddcPath),
		ServerConfig: serverConfig,
		MaxJobs:      jobs,
	}
}

func handleWSL2GameBuild(ctx context.Context, cfg *config.Config, input gameBuildInput) (*mcp.CallToolResult, any, error) {
	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash, input.Config)
	if hit := checkCacheHit(input.NoCache, cache.StageGameServer, serverHash, globals.ServerBuildOutput(cfg),
		gameBuildResult{Success: true, Output: "Game server build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	out := newToolOutput()
	w, opts, err := setupWSL2GameBuild(cfg, input, out)
	if err != nil {
		return resultErr(gameBuildResult{Error: err.Error()})
	}

	var result gameBuildResult

	br, err := wsl.BuildGame(ctx, w, opts)
	applyServerBuildResult(br, &result)
	result.Output = out.String()

	if err != nil {
		result.Error = fmt.Sprintf("WSL2 game build failed: %v", err)
		return resultErr(result)
	}

	if result.Success {
		saveCache(cache.StageGameServer, serverHash, deployToolDryRun(input.DryRun))
	}

	return resultOK(result)
}

func setupWSL2GameBuild(cfg *config.Config, input gameBuildInput, out *toolOutput) (*wsl.WSL2, wsl.GameOptions, error) {
	s, err := state.Load()
	if err != nil {
		return nil, wsl.GameOptions{}, fmt.Errorf("loading state: %v", err)
	}
	if s.WSL2Engine == nil {
		return nil, wsl.GameOptions{}, fmt.Errorf("no WSL2 engine build found; run ludus_engine_build with backend=wsl2 first")
	}

	r := newCaptureRunner(out, deployToolDryRun(input.DryRun))
	w, err := wsl.New(r, input.WSLDistro)
	if err != nil {
		return nil, wsl.GameOptions{}, fmt.Errorf("WSL2 init failed: %v\n\nIf WSL2 is not available, use Podman instead: ludus_game_build with backend=podman", err)
	}

	ddcMode, ddcPath, _, err := globals.ResolveDDC()
	if err != nil {
		return nil, wsl.GameOptions{}, fmt.Errorf("resolving DDC: %v", err)
	}

	opts := wslGameOpts(cfg, s.WSL2Engine, w, ddcMode, ddcPath, config.ResolvedServerBuildConfig(input.Config), input.Jobs)

	return w, opts, nil
}

func handleGameClient(ctx context.Context, _ *mcp.CallToolRequest, input gameClientInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg.Clone()

	platform := input.Platform
	if platform == "" {
		platform = "Linux"
	}

	// Merge the tool input into config before hashing: backend (#728) and
	// skip-cook (#558).
	globals.ApplyEngineOverrides(&cfg, input.Backend, "")
	if input.SkipCook {
		cfg.Game.SkipCook = true
	}

	be := resolveBackend(input.Backend, cfg.Engine.Backend)

	if dockerbuild.IsContainerBackend(be) {
		return handleContainerGameClient(ctx, &cfg, input, platform, be)
	}

	engineHash := cache.EngineKey(&cfg)
	clientHash := cache.GameClientKey(&cfg, engineHash, platform)
	if hit := checkCacheHit(input.NoCache, cache.StageGameClient, clientHash, globals.ClientBuildOutput(&cfg, platform),
		gameBuildResult{Success: true, Output: "Game client build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	opts, err := makeGameBuildOpts(&cfg, cfg.Game.SkipCook, platform, "", input.Jobs)
	if err != nil {
		return resultErr(gameBuildResult{Error: err.Error()})
	}
	out := newToolOutput()
	r := newCaptureRunner(out, deployToolDryRun(input.DryRun))
	b := game.NewBuilder(opts, r)

	var result gameBuildResult

	br, err := b.BuildClient(ctx)
	applyClientBuildResult(br, &result)
	result.Output = out.String()

	if err != nil {
		result.Error = fmt.Sprintf("game client build failed: %v", err)
		return resultErr(result)
	}

	persistClientBuild(deployToolDryRun(input.DryRun), platform, clientHash, &result)

	return resultOK(result)
}

func handleContainerGameClient(ctx context.Context, cfg *config.Config, input gameClientInput, platform string, be string) (*mcp.CallToolResult, any, error) {
	engineHash := cache.EngineKey(cfg)
	clientHash := cache.GameClientKey(cfg, engineHash, platform)

	out := newToolOutput()
	r := newCaptureRunner(out, deployToolDryRun(input.DryRun))
	// The shared client assembly archives under the PackagedClient root —
	// not the server root — mirroring the native client builder's layout (#750).
	opts, err := globals.ResolveContainerClientBuildOptions(cfg, be, platform)
	if err != nil {
		return resultErr(gameBuildResult{Error: err.Error()})
	}

	if hit := checkContainerCacheHit(input.NoCache, cache.StageGameClient, clientHash,
		globals.ClientBuildOutput(cfg, platform), engineImageProbe(r, ctx, be, opts.EngineImage),
		gameBuildResult{Success: true, Output: "Game client build is up to date (cached), skipping."}, out); hit != nil {
		return hit, nil, nil
	}

	b := dockerbuild.NewDockerGameBuilder(opts, r)

	var result gameBuildResult

	br, err := b.BuildClient(ctx)
	applyClientBuildResult(br, &result)
	result.Output = out.String()

	if err != nil {
		result.Error = fmt.Sprintf("container client build failed: %v", err)
		return resultErr(result)
	}

	persistClientBuild(deployToolDryRun(input.DryRun), platform, clientHash, &result)

	return resultOK(result)
}
