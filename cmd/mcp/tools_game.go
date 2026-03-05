package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/game"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type gameBuildInput struct {
	SkipCook bool   `json:"skip_cook,omitempty" jsonschema:"Skip content cooking (use previously cooked content)"`
	Backend  string `json:"backend,omitempty" jsonschema:"Build backend: native or docker (default: from config)"`
	Arch     string `json:"arch,omitempty" jsonschema:"Target CPU architecture: amd64 or arm64 (default: from config)"`
	NoCache  bool   `json:"no_cache,omitempty" jsonschema:"Disable build caching (force rebuild even if inputs are unchanged)"`
	DryRun   bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type gameClientInput struct {
	Platform string `json:"platform,omitempty" jsonschema:"Target platform: Linux or Win64"`
	SkipCook bool   `json:"skip_cook,omitempty" jsonschema:"Skip content cooking"`
	Backend  string `json:"backend,omitempty" jsonschema:"Build backend: native or docker (default: from config)"`
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
		Description: "Build the UE5 game as a Linux dedicated server via RunUAT BuildCookRun. Use backend='docker' to build inside a pre-built engine Docker image. This is a long-running operation.",
	}, handleGameBuild)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_game_client",
		Description: "Build the standalone game client for Linux or Win64 via RunUAT BuildCookRun. Use backend='docker' for Linux-only Docker builds. This is a long-running operation.",
	}, handleGameClient)
}

func makeGameBuildOpts(cfg *config.Config, skipCook bool, clientPlatform string) game.BuildOptions {
	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)

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
	}
}

// mcpResolveEngineImage determines the Docker image for game builds in MCP context.
func mcpResolveEngineImage(cfg *config.Config) (string, error) {
	if cfg.Engine.DockerImage != "" {
		return cfg.Engine.DockerImage, nil
	}

	s, err := state.Load()
	if err == nil && s.EngineImage != nil && s.EngineImage.ImageTag != "" {
		return s.EngineImage.ImageTag, nil
	}

	imageName := cfg.Engine.DockerImageName
	if imageName == "" {
		imageName = "ludus-engine"
	}
	version, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
	tag := version
	if tag == "" {
		tag = "latest"
	}
	return fmt.Sprintf("%s:%s", imageName, tag), nil
}

func handleGameBuild(ctx context.Context, _ *mcp.CallToolRequest, input gameBuildInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	// Apply arch override
	if input.Arch != "" {
		cfg.Game.Arch = input.Arch
	}

	be := input.Backend
	if be == "" {
		be = cfg.Engine.Backend
	}

	if be == "docker" {
		return handleDockerGameBuild(ctx, input)
	}

	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)
	if hit := checkCacheHit(input.NoCache, cache.StageGameServer, serverHash,
		gameBuildResult{Success: true, Output: "Game server build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	opts := makeGameBuildOpts(cfg, input.SkipCook, "")
	r := runner.NewRunner(true, input.DryRun || globals.DryRun)
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
	result.Output = captured.Stdout + captured.Stderr

	if err != nil {
		result.Error = fmt.Sprintf("game server build failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(result)},
			},
		}, nil, nil
	}

	if result.Success {
		if c, cErr := cache.Load(); cErr == nil {
			c.Set(cache.StageGameServer, serverHash, time.Now().UTC().Format(time.RFC3339))
			_ = cache.Save(c)
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}

func handleDockerGameBuild(ctx context.Context, input gameBuildInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)
	if hit := checkCacheHit(input.NoCache, cache.StageGameServer, serverHash,
		gameBuildResult{Success: true, Output: "Game server build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	r := runner.NewRunner(true, input.DryRun || globals.DryRun)

	engineImage, err := mcpResolveEngineImage(cfg)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(gameBuildResult{Error: err.Error()})},
			},
		}, nil, nil
	}

	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
	b := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
		EngineImage:   engineImage,
		ProjectPath:   cfg.Game.ProjectPath,
		ProjectName:   cfg.Game.ProjectName,
		ServerTarget:  cfg.Game.ResolvedServerTarget(),
		GameTarget:    cfg.Game.ResolvedGameTarget(),
		SkipCook:      input.SkipCook,
		ServerMap:     cfg.Game.ServerMap,
		EngineVersion: engineVersion,
	}, r)

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
	result.Output = captured.Stdout + captured.Stderr

	if err != nil {
		result.Error = fmt.Sprintf("docker game build failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(result)},
			},
		}, nil, nil
	}

	if result.Success {
		if c, cErr := cache.Load(); cErr == nil {
			c.Set(cache.StageGameServer, serverHash, time.Now().UTC().Format(time.RFC3339))
			_ = cache.Save(c)
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}

func handleGameClient(ctx context.Context, _ *mcp.CallToolRequest, input gameClientInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	platform := input.Platform
	if platform == "" {
		platform = "Linux"
	}

	be := input.Backend
	if be == "" {
		be = cfg.Engine.Backend
	}

	if be == "docker" {
		return handleDockerGameClient(ctx, input, platform)
	}

	engineHash := cache.EngineKey(cfg)
	clientHash := cache.GameClientKey(cfg, engineHash, platform)
	if hit := checkCacheHit(input.NoCache, cache.StageGameClient, clientHash,
		gameBuildResult{Success: true, Output: "Game client build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	opts := makeGameBuildOpts(cfg, input.SkipCook, platform)
	r := runner.NewRunner(true, input.DryRun || globals.DryRun)
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
	result.Output = captured.Stdout + captured.Stderr

	if err != nil {
		result.Error = fmt.Sprintf("game client build failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(result)},
			},
		}, nil, nil
	}

	// Persist client build info to state
	if result.Success {
		_ = state.UpdateClient(&state.ClientState{
			BinaryPath: result.Binary,
			OutputDir:  result.OutputDir,
			Platform:   platform,
			BuiltAt:    time.Now().UTC().Format(time.RFC3339),
		})
		if c, cErr := cache.Load(); cErr == nil {
			c.Set(cache.StageGameClient, clientHash, time.Now().UTC().Format(time.RFC3339))
			_ = cache.Save(c)
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}

func handleDockerGameClient(ctx context.Context, input gameClientInput, platform string) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	engineHash := cache.EngineKey(cfg)
	clientHash := cache.GameClientKey(cfg, engineHash, platform)
	if hit := checkCacheHit(input.NoCache, cache.StageGameClient, clientHash,
		gameBuildResult{Success: true, Output: "Game client build is up to date (cached), skipping."}); hit != nil {
		return hit, nil, nil
	}

	r := runner.NewRunner(true, input.DryRun || globals.DryRun)

	engineImage, err := mcpResolveEngineImage(cfg)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(gameBuildResult{Error: err.Error()})},
			},
		}, nil, nil
	}

	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
	b := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
		EngineImage:    engineImage,
		ProjectPath:    cfg.Game.ProjectPath,
		ProjectName:    cfg.Game.ProjectName,
		ClientTarget:   cfg.Game.ResolvedClientTarget(),
		ClientPlatform: platform,
		SkipCook:       input.SkipCook,
		EngineVersion:  engineVersion,
	}, r)

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
	result.Output = captured.Stdout + captured.Stderr

	if err != nil {
		result.Error = fmt.Sprintf("docker client build failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(result)},
			},
		}, nil, nil
	}

	if result.Success {
		_ = state.UpdateClient(&state.ClientState{
			BinaryPath: result.Binary,
			OutputDir:  result.OutputDir,
			Platform:   platform,
			BuiltAt:    time.Now().UTC().Format(time.RFC3339),
		})
		if c, cErr := cache.Load(); cErr == nil {
			c.Set(cache.StageGameClient, clientHash, time.Now().UTC().Format(time.RFC3339))
			_ = cache.Save(c)
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}
