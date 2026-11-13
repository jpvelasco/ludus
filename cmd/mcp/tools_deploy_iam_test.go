package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDeployFleetRejectsPathBeforeResolution(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "dry run"}[dryRun], func(t *testing.T) {
			cfg := config.Defaults()
			cfg.GameLift.IAM.Path = "/project/"
			globals.SetGlobals(t, cfg)
			globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
				t.Fatal("invalid path reached target resolution")
				return nil, nil
			})
			result, _, err := handleDeployFleet(context.Background(), nil, deployFleetInput{DryRun: dryRun})
			if err != nil || result == nil || !result.IsError {
				t.Fatalf("expected tool error, got %v, %v", result, err)
			}
			text := result.Content[0].(*mcpsdk.TextContent).Text
			if !strings.Contains(text, "root-path role") {
				t.Fatalf("missing path guidance: %s", text)
			}
		})
	}
}
