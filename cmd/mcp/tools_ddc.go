package mcp

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/ddc"
	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/toolchain"
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
	Mode      string `json:"mode,omitempty" jsonschema:"DDC mode: local or none"`
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
		Description: "Show current DDC backend, path, and cache size on disk.",
	}, handleDDCStatus)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "ludus_ddc_clean",
		Description: "Delete all DDC cache content, freeing disk space.",
	}, handleDDCClean)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "ludus_ddc_configure",
		Description: "Set DDC mode and path in ludus.yaml. Changes are validated and persisted to disk.",
	}, handleDDCConfigure)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "ludus_ddc_warm",
		Description: "Run a cook-only Docker build to pre-populate the DDC with shaders and derived data. Set dry_run to preview without executing.",
	}, handleDDCWarm)
}

func handleDDCStatus(ctx context.Context, _ *mcpsdk.CallToolRequest, _ ddcStatusInput) (*mcpsdk.CallToolResult, any, error) {
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

func handleDDCClean(ctx context.Context, _ *mcpsdk.CallToolRequest, _ ddcCleanInput) (*mcpsdk.CallToolResult, any, error) {
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

func handleDDCConfigure(ctx context.Context, _ *mcpsdk.CallToolRequest, input ddcConfigureInput) (*mcpsdk.CallToolResult, any, error) {
	changed := false

	if input.Mode != "" {
		validated, err := ddc.ValidateDDCMode(input.Mode)
		if err != nil {
			return toolError(err.Error())
		}
		viper.Set("ddc.mode", validated)
		globals.Cfg.DDC.Mode = validated
		changed = true
	}

	if input.LocalPath != "" {
		viper.Set("ddc.local_path", input.LocalPath)
		globals.Cfg.DDC.LocalPath = input.LocalPath
		changed = true
	}

	if changed {
		if err := viper.WriteConfig(); err != nil {
			return toolError(fmt.Sprintf("persisting DDC config to ludus.yaml: %v", err))
		}
	}

	mode := globals.ResolveDDCMode()
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
	cfg := globals.Cfg
	mode := globals.ResolveDDCMode()

	if mode == "none" {
		return resultOK(ddcWarmResult{
			Success: false,
			Message: "DDC mode is 'none'; warmup requires 'local' mode",
		})
	}

	if cfg.Engine.Backend != "docker" && cfg.Engine.DockerImage == "" {
		return resultOK(ddcWarmResult{
			Success: false,
			Message: "DDC warmup requires Docker backend (set engine.backend: docker in ludus.yaml)",
		})
	}

	ddcPath, err := ddc.ResolvePath(cfg.DDC.LocalPath)
	if err != nil {
		return toolError(fmt.Sprintf("resolving DDC path: %v", err))
	}

	engineImage := resolveWarmupEngineImage(cfg)

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

func resolveWarmupEngineImage(cfg *config.Config) string {
	if cfg.Engine.DockerImage != "" {
		return cfg.Engine.DockerImage
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
	return fmt.Sprintf("%s:%s", imageName, tag)
}
