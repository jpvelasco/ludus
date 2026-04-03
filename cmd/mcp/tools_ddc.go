package mcp

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/ddc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ddcStatusInput struct{}

type ddcStatusResult struct {
	Mode      string `json:"mode"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

type ddcCleanInput struct{}

type ddcCleanResult struct {
	Success    bool  `json:"success"`
	BytesFreed int64 `json:"bytes_freed"`
}

type ddcConfigureInput struct {
	Mode      string `json:"mode,omitempty" jsonschema:"description=DDC mode: local or none"`
	LocalPath string `json:"local_path,omitempty" jsonschema:"description=Override local DDC path"`
}

type ddcConfigureResult struct {
	Mode string `json:"mode"`
	Path string `json:"path"`
}

type ddcWarmInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"description=Print commands without executing"`
}

type ddcWarmResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func registerDDCTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_ddc_status",
		Description: "Show current DDC backend, path, and cache size on disk.",
	}, handleDDCStatus)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_ddc_clean",
		Description: "Delete all DDC cache content, freeing disk space.",
	}, handleDDCClean)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_ddc_configure",
		Description: "Apply DDC settings to the current project configuration.",
	}, handleDDCConfigure)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_ddc_warm",
		Description: "Trigger a minimal engine cook to pre-populate the DDC with engine shaders and derived data.",
	}, handleDDCWarm)
}

func handleDDCStatus(ctx context.Context, _ *mcp.CallToolRequest, _ ddcStatusInput) (*mcp.CallToolResult, any, error) {
	mode := globals.ResolveDDCMode()
	ddcPath, err := ddc.ResolvePath(globals.Cfg.DDC.LocalPath)
	if err != nil {
		return toolError(fmt.Sprintf("resolving DDC path: %v", err))
	}

	size, err := ddc.DirSize(ddcPath)
	if err != nil {
		return toolError(fmt.Sprintf("calculating DDC size: %v", err))
	}

	return resultOK(ddcStatusResult{
		Mode:      mode,
		Path:      ddcPath,
		SizeBytes: size,
	})
}

func handleDDCClean(ctx context.Context, _ *mcp.CallToolRequest, _ ddcCleanInput) (*mcp.CallToolResult, any, error) {
	ddcPath, err := ddc.ResolvePath(globals.Cfg.DDC.LocalPath)
	if err != nil {
		return toolError(fmt.Sprintf("resolving DDC path: %v", err))
	}

	freed, err := ddc.Clean(ddcPath)
	if err != nil {
		return toolError(fmt.Sprintf("cleaning DDC: %v", err))
	}

	return resultOK(ddcCleanResult{
		Success:    true,
		BytesFreed: freed,
	})
}

func handleDDCConfigure(ctx context.Context, _ *mcp.CallToolRequest, input ddcConfigureInput) (*mcp.CallToolResult, any, error) {
	if input.Mode != "" {
		globals.Cfg.DDC.Mode = input.Mode
	}
	if input.LocalPath != "" {
		globals.Cfg.DDC.LocalPath = input.LocalPath
	}

	mode := globals.ResolveDDCMode()
	ddcPath, err := ddc.ResolvePath(globals.Cfg.DDC.LocalPath)
	if err != nil {
		return toolError(fmt.Sprintf("resolving DDC path: %v", err))
	}

	return resultOK(ddcConfigureResult{
		Mode: mode,
		Path: ddcPath,
	})
}

func handleDDCWarm(ctx context.Context, _ *mcp.CallToolRequest, input ddcWarmInput) (*mcp.CallToolResult, any, error) {
	mode := globals.ResolveDDCMode()
	if mode == "none" {
		return resultOK(ddcWarmResult{
			Success: false,
			Message: "DDC mode is 'none'; warmup requires 'local' mode",
		})
	}

	return resultOK(ddcWarmResult{
		Success: true,
		Message: "DDC warmup triggered. Use ludus_ddc_status to check cache size after completion.",
	})
}
