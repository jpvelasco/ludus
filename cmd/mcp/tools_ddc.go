package mcp

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
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
	validated, err := validateConfigureMode(input.Mode)
	if err != nil {
		return toolError(err.Error())
	}

	changed, err := persistDDCConfig(input, validated)
	if err != nil {
		return toolError(err.Error())
	}

	return resolveDDCConfigResult(changed)
}

func validateConfigureMode(mode string) (string, error) {
	if mode == "" {
		return "", nil
	}
	return ddc.ValidateDDCMode(mode)
}

func persistDDCConfig(input ddcConfigureInput, validated string) (bool, error) {
	oldMode := viper.GetString("ddc.mode")
	oldPath := viper.GetString("ddc.localPath")

	changed := false
	if input.Mode != "" {
		viper.Set("ddc.mode", validated)
		changed = true
	}
	if input.LocalPath != "" {
		viper.Set("ddc.localPath", input.LocalPath)
		changed = true
	}
	if !changed {
		return false, nil
	}

	if err := viper.WriteConfig(); err != nil {
		viper.Set("ddc.mode", oldMode)
		viper.Set("ddc.localPath", oldPath)
		return false, fmt.Errorf("failed to save DDC config to ludus.yaml: %w; check file permissions and ensure ludus.yaml exists", err)
	}

	applyDDCConfig(input, validated)
	return true, nil
}

func applyDDCConfig(input ddcConfigureInput, validated string) {
	if input.Mode != "" {
		globals.Cfg.DDC.Mode = validated
	}
	if input.LocalPath != "" {
		globals.Cfg.DDC.LocalPath = input.LocalPath
	}
}

func resolveDDCConfigResult(changed bool) (*mcpsdk.CallToolResult, any, error) {
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
	mode, ddcPath, engineImage, err := validateWarmPrereqs(cfg)
	if err != nil {
		return toolError(err.Error())
	}

	if input.DryRun {
		return resultOK(ddcWarmResult{
			Success: true,
			Message: fmt.Sprintf("Would run DDC warmup:\n  Image: %s\n  Project: %s\n  DDC path: %s\n  Flags: -cook -skipbuild -NoCompile -NoCompileEditor -NoP4 -map=MinimalDefaultMap",
				engineImage, cfg.Game.ProjectName, ddcPath),
		})
	}

	return executeMCPWarmup(ctx, cfg, mode, ddcPath, engineImage)
}

func validateWarmPrereqs(cfg config.Config) (mode, ddcPath, engineImage string, err error) {
	mode, err = globals.ResolveDDCMode()
	if err != nil {
		return "", "", "", err
	}
	if mode == "none" {
		return "", "", "", fmt.Errorf("DDC warmup requires 'local' mode (current mode: none)")
	}
	if cfg.Engine.Backend != "docker" && cfg.Engine.DockerImage == "" {
		return "", "", "", fmt.Errorf("DDC warmup requires Docker backend (set engine.backend: docker in ludus.yaml)")
	}
	ddcPath, err = ddc.ResolvePath(cfg.DDC.LocalPath)
	if err != nil {
		return "", "", "", fmt.Errorf("resolving DDC path: %w", err)
	}
	engineImage, err = globals.ResolveWarmupEngineImage(&cfg)
	if err != nil {
		return "", "", "", err
	}
	return mode, ddcPath, engineImage, nil
}

func executeMCPWarmup(ctx context.Context, cfg config.Config, mode, ddcPath, engineImage string) (*mcpsdk.CallToolResult, any, error) {
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

	captured, err := withCapture(func() error {
		_, buildErr := builder.Build(ctx)
		return buildErr
	})

	if err != nil {
		return toolError(fmt.Sprintf("DDC warmup failed: %v\n%s", err, mergeOutput(captured)))
	}

	return resultOK(ddcWarmResult{
		Success: true,
		Message: "DDC warmup complete",
	})
}
