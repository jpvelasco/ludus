package mcp

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/container"
	"github.com/devrecon/ludus/internal/ecr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type containerBuildInput struct {
	Tag     string `json:"tag,omitempty" jsonschema:"Docker image tag (default: from config or latest)"`
	NoCache bool   `json:"no_cache,omitempty" jsonschema:"Disable Docker build cache"`
	Arch    string `json:"arch,omitempty" jsonschema:"Target CPU architecture: amd64 or arm64 (default: from config)"`
	DryRun  bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type containerPushInput struct {
	Tag    string `json:"tag,omitempty" jsonschema:"Docker image tag to push (default: from config or latest)"`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

type containerResult struct {
	Success         bool    `json:"success"`
	ImageTag        string  `json:"image_tag,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	Output          string  `json:"output,omitempty"`
	Error           string  `json:"error,omitempty"`
}

func registerContainerTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_container_build",
		Description: "Build a Docker container image for the dedicated server. Generates Dockerfile, builds GameLift wrapper, and runs docker build.",
	}, handleContainerBuild)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_container_push",
		Description: "Push the container image to Amazon ECR. Authenticates with ECR, creates repository if needed, tags and pushes the image.",
	}, handleContainerPush)
}

func handleContainerBuild(ctx context.Context, _ *mcp.CallToolRequest, input containerBuildInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg.Clone()
	r := newToolRunner(input.DryRun)

	applyArchOverride(&cfg, input.Arch)

	tag := input.Tag
	if tag == "" {
		tag = cfg.Container.Tag
	}

	serverBuildDir := config.ResolveServerBuildDir(&cfg)

	b := container.NewBuilder(container.BuildOptions{
		ServerBuildDir: serverBuildDir,
		ImageName:      cfg.Container.ImageName,
		Tag:            tag,
		ServerPort:     cfg.Container.ServerPort,
		NoCache:        input.NoCache,
		ProjectName:    cfg.Game.ProjectName,
		ServerTarget:   cfg.Game.ResolvedServerTarget(),
		Arch:           cfg.Game.ResolvedArch(),
	}, r)

	var result containerResult

	captured, err := withCapture(func() error {
		br, buildErr := b.Build(ctx)
		if br != nil {
			result.Success = br.Success
			result.ImageTag = br.ImageTag
			result.DurationSeconds = br.Duration
		}
		return buildErr
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = fmt.Sprintf("container build failed: %v", err)
		return resultErr(result)
	}

	return resultOK(result)
}

func handleContainerPush(ctx context.Context, _ *mcp.CallToolRequest, input containerPushInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg
	r := newToolRunner(input.DryRun)

	tag := input.Tag
	if tag == "" {
		tag = cfg.Container.Tag
	}

	serverBuildDir := config.ResolveServerBuildDir(cfg)

	b := container.NewBuilder(container.BuildOptions{
		ServerBuildDir: serverBuildDir,
		ImageName:      cfg.Container.ImageName,
		Tag:            tag,
		ServerPort:     cfg.Container.ServerPort,
		ProjectName:    cfg.Game.ProjectName,
		ServerTarget:   cfg.Game.ResolvedServerTarget(),
		Arch:           cfg.Game.ResolvedArch(),
	}, r)

	var result containerResult
	result.ImageTag = fmt.Sprintf("%s:%s", cfg.Container.ImageName, tag)

	captured, err := withCapture(func() error {
		return b.Push(ctx, ecr.PushOptions{
			ECRRepository: cfg.AWS.ECRRepository,
			AWSRegion:     cfg.AWS.Region,
			AWSAccountID:  cfg.AWS.AccountID,
			ImageTag:      tag,
		})
	})
	result.Output = mergeOutput(captured)

	if err != nil {
		result.Error = fmt.Sprintf("container push failed: %v", err)
		return resultErr(result)
	}

	result.Success = true
	return resultOK(result)
}
