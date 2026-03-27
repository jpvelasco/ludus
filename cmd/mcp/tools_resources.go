package mcp

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/awsutil"
	"github.com/devrecon/ludus/internal/inventory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type resourcesInput struct {
	Region string `json:"region,omitempty" jsonschema:"AWS region override"`
}

func registerResourcesTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_resources",
		Description: "List all ludus-managed AWS resources in the configured region. Discovers resources by ManagedBy=ludus tag and known naming patterns (ECR repositories, S3 build buckets).",
	}, handleResources)
}

func handleResources(ctx context.Context, _ *mcp.CallToolRequest, input resourcesInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	region := input.Region
	if region == "" {
		region = cfg.AWS.Region
	}

	awsCfg, err := awsutil.LoadAWSConfig(ctx, region)
	if err != nil {
		return toolError(fmt.Sprintf("could not load AWS config: %v", err))
	}

	ecrRepo := cfg.AWS.ECRRepository
	if ecrRepo == "" {
		ecrRepo = "ludus-server"
	}
	engineRepo := cfg.Engine.DockerImageName
	if engineRepo == "" {
		engineRepo = "ludus-engine"
	}
	ecrRepoNames := []string{ecrRepo}
	if engineRepo != ecrRepo {
		ecrRepoNames = append(ecrRepoNames, engineRepo)
	}

	scanner := inventory.NewScanner(awsCfg, region, ecrRepoNames, "ludus-builds-")
	inv, err := scanner.Scan(ctx)
	if err != nil {
		return toolError(fmt.Sprintf("scanning resources: %v", err))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: jsonString(inv)},
		},
	}, nil, nil
}
