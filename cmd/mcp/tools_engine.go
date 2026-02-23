package mcp

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/engine"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type engineSetupInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type engineBuildInput struct {
	Jobs   int  `json:"jobs,omitempty" jsonschema:"Max parallel compile jobs (0 = auto-detect from RAM)"`
	DryRun bool `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type engineResult struct {
	Success         bool    `json:"success"`
	EnginePath      string  `json:"engine_path,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	Output          string  `json:"output,omitempty"`
	Error           string  `json:"error,omitempty"`
}

func registerEngineTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_engine_setup",
		Description: "Run Setup.sh to download Unreal Engine dependencies. Must be run before engine build.",
	}, handleEngineSetup)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_engine_build",
		Description: "Build Unreal Engine from source. Runs Setup, GenerateProjectFiles, and compiles ShaderCompileWorker + UnrealEditor. This is a long-running operation.",
	}, handleEngineBuild)
}

func handleEngineSetup(ctx context.Context, _ *mcp.CallToolRequest, input engineSetupInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg
	r := runner.NewRunner(true, input.DryRun || globals.DryRun)

	b := engine.NewBuilder(engine.BuildOptions{
		SourcePath: cfg.Engine.SourcePath,
		Verbose:    true,
	}, r)

	var result engineResult
	result.EnginePath = cfg.Engine.SourcePath

	captured, err := withCapture(func() error {
		return b.Setup(ctx)
	})
	result.Output = captured.Stdout + captured.Stderr

	if err != nil {
		result.Error = err.Error()
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(result)},
			},
		}, nil, nil
	}

	result.Success = true
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(result)},
		},
	}, nil, nil
}

func handleEngineBuild(ctx context.Context, _ *mcp.CallToolRequest, input engineBuildInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg
	r := runner.NewRunner(true, input.DryRun || globals.DryRun)

	jobs := input.Jobs
	if jobs == 0 {
		jobs = cfg.Engine.MaxJobs
	}

	b := engine.NewBuilder(engine.BuildOptions{
		SourcePath: cfg.Engine.SourcePath,
		MaxJobs:    jobs,
		Verbose:    true,
	}, r)

	var result engineResult
	result.EnginePath = cfg.Engine.SourcePath

	captured, err := withCapture(func() error {
		br, buildErr := b.Build(ctx)
		if br != nil {
			result.DurationSeconds = br.Duration
			result.Success = br.Success
		}
		return buildErr
	})
	result.Output = captured.Stdout + captured.Stderr

	if err != nil {
		result.Error = fmt.Sprintf("engine build failed: %v", err)
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
