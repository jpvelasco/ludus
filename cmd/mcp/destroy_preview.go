package mcp

import (
	"fmt"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func previewDeployDestroy(cfg *config.Config, input deployDestroyInput) (*mcp.CallToolResult, any, error) {
	st, err := state.Load()
	if err != nil {
		return toolError(fmt.Sprintf("loading destroy preview state: %v", err))
	}
	plan, err := globals.PlanDestroy(cfg, st, state.ActiveProfile(), input.Target, input.AllTargets, input.Purge)
	result := deployDestroyResult{Success: err == nil, Plan: plan}
	if plan != nil {
		result.Output = deploy.RenderDestroyPlan(plan)
	}
	if err != nil {
		result.Error = err.Error()
		return resultErr(result)
	}
	return resultOK(result)
}
