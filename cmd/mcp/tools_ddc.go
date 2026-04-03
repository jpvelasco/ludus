package mcp

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/ddc"
	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/runner"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/viper"
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
	Mode      string `json:"mode,omitempty" jsonschema:"DDC mode: \"local\" (persistent filesystem cache) or \"none\" (disable DDC)"`
	LocalPath string `json:"local_path,omitempty" jsonschema:"Override local DDC path"`
}

type ddcConfigureResult struct {
	Mode      string `json:"mode"`
	Path      string `json:"path"`
	Persisted bool   `json:"persisted"`
}

type ddcWarmInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"Preview the warmup without executing"`
}

type ddcWarmResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func registerDDCTools(s *mcpsdk.Server) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "ludus_ddc_status",
		Description: "Show current DDC mode (local/none), storage path, and cache size on disk.",
	}, handleDDCStatus)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "ludus_ddc_clean",
		Description: "Delete all DDC cache content, freeing disk space.",
	}, handleDDCClean)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "ludus_ddc_configure",
		Description: "Set DDC mode and/or path in ludus.yaml. Both parameters are optional; omitted fields are left unchanged. Changes are validated before persisting.",
	}, handleDDCConfigure)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "ludus_ddc_warm",
		Description: "Run a cook-only Docker build to pre-populate the DDC with shaders and derived data. Set dry_run to preview without executing.",
	}, handleDDCWarm)
}

func handleDDCStatus(ctx context.Context, _ *mcpsdk.CallToolRequest, _ ddcStatusInput) (*mcpsdk.CallToolResult, any, error) {
	cfg := globals.Cfg.Clone()
	mode, err := globals.ResolveDDCMode()
	if err != nil {
		return toolError(err.Error())
	}
	ddcPath, err := ddc.ResolvePath(cfg.DDC.LocalPath)
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

func handleDDCClean(ctx context.Context, _ *mcpsdk.CallToolRequest, _ ddcCleanInput) (*mcpsdk.CallToolResult, any, error) {
	cfg := globals.Cfg.Clone()
	ddcPath, err := ddc.ResolvePath(cfg.DDC.LocalPath)
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

func handleDDCConfigure(ctx context.Context, _ *mcpsdk.CallToolRequest, input ddcConfigureInput) (*mcpsdk.CallToolResult, any, error) {
	// Validate mode before touching any state.
	var validated string
	if input.Mode != "" {
		var err error
		validated, err = ddc.ValidateDDCMode(input.Mode)
		if err != nil {
			return toolError(err.Error())
		}
	}

	// Store old values for rollback on write failure.
	oldMode := viper.GetString("ddc.mode")
	oldPath := viper.GetString("ddc.local_path")

	changed := false
	if input.Mode != "" {
		viper.Set("ddc.mode", validated)
		changed = true
	}
	if input.LocalPath != "" {
		viper.Set("ddc.local_path", input.LocalPath)
		changed = true
	}

	if changed {
		if err := viper.WriteConfig(); err != nil {
			// Rollback viper state so in-memory matches disk.
			viper.Set("ddc.mode", oldMode)
			viper.Set("ddc.local_path", oldPath)
			return toolError(fmt.Sprintf("persisting DDC config to ludus.yaml: %v", err))
		}
		// Only update in-memory config after successful persist.
		if input.Mode != "" {
			globals.Cfg.DDC.Mode = validated
		}
		if input.LocalPath != "" {
			globals.Cfg.DDC.LocalPath = input.LocalPath
		}
	}

	mode, err := globals.ResolveDDCMode()
	if err != nil {
		return toolError(err.Error())
	}
	ddcPath, err := ddc.ResolvePath(globals.Cfg.DDC.LocalPath)
	if err != nil {
		return toolError(fmt.Sprintf("resolving DDC path: %v", err))
	}

	return resultOK(ddcConfigureResult{
		Mode:      mode,
		Path:      ddcPath,
		Persisted: changed,
	})
}

func handleDDCWarm(ctx context.Context, _ *mcpsdk.CallToolRequest, input ddcWarmInput) (*mcpsdk.CallToolResult, any, error) {
	cfg := globals.Cfg.Clone()
	mode, err := globals.ResolveDDCMode()
	if err != nil {
		return toolError(err.Error())
	}

	if mode == "none" {
		return toolError("DDC warmup requires 'local' mode (current mode: none)")
	}

	if cfg.Engine.Backend != "docker" && cfg.Engine.DockerImage == "" {
		return toolError("DDC warmup requires Docker backend (set engine.backend: docker in ludus.yaml)")
	}

	ddcPath, err := ddc.ResolvePath(cfg.DDC.LocalPath)
	if err != nil {
		return toolError(fmt.Sprintf("resolving DDC path: %v", err))
	}

	engineImage, err := globals.ResolveWarmupEngineImage(&cfg)
	if err != nil {
		return toolError(err.Error())
	}

	if input.DryRun {
		return resultOK(ddcWarmResult{
			Success: true,
			Message: fmt.Sprintf("Would run cook-only build: image=%s, project=%s, ddc=%s",
				engineImage, cfg.Game.ProjectName, ddcPath),
		})
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
		EngineImage:   engineImage,
		ProjectPath:   cfg.Game.ProjectPath,
		ProjectName:   cfg.Game.ProjectName,
		EngineVersion: cfg.Engine.Version,
		DDCMode:       mode,
		DDCPath:       ddcPath,
		CookOnly:      true,
	}, r)

	if _, err := builder.Build(ctx); err != nil {
		return toolError(fmt.Sprintf("DDC warmup failed: %v", err))
	}

	return resultOK(ddcWarmResult{
		Success: true,
		Message: "DDC warmup complete",
	})
}
