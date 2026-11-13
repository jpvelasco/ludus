package mcp

import (
	"context"
	"fmt"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/inventory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type resourcesInput struct {
	Region string `json:"region,omitempty" jsonschema:"AWS region override"`
}

func registerResourcesTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_resources",
		Description: "List project-tagged AWS resources and configured repositories in the configured region. Matches the configured ManagedBy and Project tags; also lists configured ECR repositories (configured names do not prove ownership).",
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

	scanner := inventory.NewScanner(awsCfg, inventory.OptionsForConfig(cfg, region))
	inv, err := scanner.Scan(ctx)
	if err != nil {
		return toolError(fmt.Sprintf("scanning resources: %v", err))
	}

	return resultOK(inv)
}
