package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/game"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type gameBuildInput struct {
	SkipCook bool `json:"skip_cook,omitempty" jsonschema:"Skip content cooking (use previously cooked content)"`
	DryRun   bool `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type gameClientInput struct {
	Platform string `json:"platform,omitempty" jsonschema:"Target platform: Linux or Win64"`
	SkipCook bool   `json:"skip_cook,omitempty" jsonschema:"Skip content cooking"`
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
		Description: "Build the UE5 game as a Linux dedicated server via RunUAT BuildCookRun. This is a long-running operation.",
	}, handleGameBuild)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_game_client",
		Description: "Build the standalone game client for Linux or Win64 via RunUAT BuildCookRun. This is a long-running operation.",
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
		ClientPlatform: clientPlatform,
		SkipCook:       skipCook,
		ServerMap:      cfg.Game.ServerMap,
		EngineVersion:  engineVersion,
	}
}

func handleGameBuild(ctx context.Context, _ *mcp.CallToolRequest, input gameBuildInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

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
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}
