package mcp

import (
	"context"
	"fmt"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ddcStatusInput struct{}

type ddcStatusResult struct {
	Mode      string `json:"mode"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

type ddcCleanInput struct{}

type ddcCleanResult struct {
	Success    bool   `json:"success"`
	BytesFreed int64  `json:"bytes_freed"`
	DryRun     bool   `json:"dry_run,omitempty"`
	Message    string `json:"message,omitempty"`
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
		Description: "Show current DDC mode (zen/local/none), the storage path for the active backend, and cache size on disk.",
	}, handleDDCStatus)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "ludus_ddc_clean",
		Description: "Delete all DDC cache content for the active backend (zen store reset or legacy cache), freeing disk space. Honors the process dry-run flag with a preview that removes nothing.",
	}, handleDDCClean)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "ludus_ddc_configure",
		Description: "Set DDC mode and/or path in ludus.yaml. Both parameters are optional; omitted fields are left unchanged. Changes are validated before persisting.",
	}, handleDDCConfigure)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "ludus_ddc_warm",
		Description: "Run a cook-only container build to pre-populate the DDC with shaders and derived data. Set dry_run to preview without executing.",
	}, handleDDCWarm)
}

func handleDDCStatus(ctx context.Context, _ *mcpsdk.CallToolRequest, _ ddcStatusInput) (*mcpsdk.CallToolResult, any, error) {
	mode, ddcPath, err := globals.ResolveActiveDDC()
	if err != nil {
		return toolError(err.Error())
	}

	// DirSize("") would walk cwd on Linux/macOS; skip when DDC is disabled.
	var size int64
	if ddcPath != "" {
		size, err = ddc.DirSize(ddcPath)
		if err != nil {
			return toolError(fmt.Sprintf("calculating DDC size: %v", err))
		}
	}

	return resultOK(ddcStatusResult{
		Mode:      mode,
		Path:      ddcPath,
		SizeBytes: size,
	})
}

func handleDDCClean(ctx context.Context, _ *mcpsdk.CallToolRequest, _ ddcCleanInput) (*mcpsdk.CallToolResult, any, error) {
	mode, ddcPath, err := globals.ResolveActiveDDC()
	if err != nil {
		return toolError(err.Error())
	}
	if mode == ddc.ModeNone {
		return resultOK(ddcCleanResult{
			Success: true,
			Message: "DDC is disabled; nothing to clean.",
		})
	}
	if globals.DryRun {
		return mcpCleanPreview(ddcPath)
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

// mcpCleanPreview reports what clean would free when the process runs with
// dry-run enabled, without touching the cache (#700).
func mcpCleanPreview(ddcPath string) (*mcpsdk.CallToolResult, any, error) {
	size, err := ddc.DirSize(ddcPath)
	if err != nil {
		return toolError(fmt.Sprintf("calculating DDC size: %v", err))
	}
	return resultOK(ddcCleanResult{
		Success:    true,
		BytesFreed: size,
		DryRun:     true,
		Message:    fmt.Sprintf("dry-run: would free %d bytes from %s; no files were removed", size, ddcPath),
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

// persistDDCConfig writes the requested DDC settings to the effective config
// file and applies them to the in-memory config only after the write
// succeeds (#701). It goes through an explicit Viper instance bound to the
// selected file: the package-global Viper is never initialized in a normal
// MCP process (config.Load builds its own instance), so WriteConfig on it
// always fails even when the file exists.
func persistDDCConfig(input ddcConfigureInput, validated string) (bool, error) {
	// Validate all inputs before touching the config file.
	if input.LocalPath != "" {
		if _, err := ddc.ResolvePath(input.LocalPath); err != nil {
			return false, err
		}
	}
	if input.Mode == "" && input.LocalPath == "" {
		return false, nil
	}

	cfgFile := globals.ResolveConfigFile()
	if err := writeDDCConfigKeys(cfgFile, input, validated); err != nil {
		return false, fmt.Errorf("failed to save DDC config to %s: %w", cfgFile, err)
	}

	applyDDCConfig(input, validated)
	return true, nil
}

// writeDDCConfigKeys applies the requested DDC keys to the config document at
// cfgFile, preserving every other field, and saves it. The in-memory config
// is only mutated by the caller once this succeeds.
func writeDDCConfigKeys(cfgFile string, input ddcConfigureInput, validated string) error {
	doc, _, err := globals.NewConfigDocument(cfgFile)
	if err != nil {
		return err
	}
	if input.Mode != "" {
		doc.Set("ddc.mode", validated)
	}
	if input.LocalPath != "" {
		doc.Set("ddc.localPath", input.LocalPath)
	}
	return globals.SaveConfigDocument(doc, cfgFile)
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
	mode, ddcPath, err := globals.ResolveActiveDDC()
	if err != nil {
		return toolError(err.Error())
	}
	return resultOK(ddcConfigureResult{
		Mode:      mode,
		Path:      ddcPath,
		Persisted: changed,
	})
}

func handleDDCWarm(ctx context.Context, _ *mcpsdk.CallToolRequest, input ddcWarmInput) (*mcpsdk.CallToolResult, any, error) {
	cfg := globals.Cfg.Clone()
	mode, ddcPath, ddcZenPath, engineImage, err := validateWarmPrereqs(cfg)
	if err != nil {
		return toolError(err.Error())
	}

	if input.DryRun {
		return resultOK(ddcWarmResult{
			Success: true,
			Message: fmt.Sprintf("Would run DDC warmup:\n  Image: %s\n  Project: %s\n  DDC path: %s\n  ZenStore path: %s\n  Flags: -cook -skipbuild -NoCompile -NoCompileEditor -NoP4 -map=MinimalDefaultMap",
				engineImage, cfg.Game.ProjectName, ddcPath, ddcZenPath),
		})
	}

	return executeMCPWarmup(ctx, cfg, mode, ddcPath, ddcZenPath, engineImage)
}

func validateWarmPrereqs(cfg config.Config) (mode, ddcPath, ddcZenPath, engineImage string, err error) {
	mode, ddcPath, ddcZenPath, err = globals.ResolveDDC()
	if err != nil {
		return "", "", "", "", err
	}
	if mode == ddc.ModeNone {
		return "", "", "", "", fmt.Errorf("DDC warmup requires 'local' mode (current mode: none)")
	}
	if !dockerbuild.IsContainerBackend(cfg.Engine.Backend) && cfg.Engine.DockerImage == "" {
		return "", "", "", "", fmt.Errorf("DDC warmup requires a container backend (set engine.backend to podman or docker in ludus.yaml)")
	}
	engineImage, err = globals.ResolveEngineImage(&cfg, true)
	if err != nil {
		return "", "", "", "", err
	}
	return mode, ddcPath, ddcZenPath, engineImage, nil
}

func executeMCPWarmup(ctx context.Context, cfg config.Config, mode, ddcPath, ddcZenPath, engineImage string) (*mcpsdk.CallToolResult, any, error) {
	out := newToolOutput()
	r := newCaptureRunner(out, globals.DryRun)
	opts := globals.BaseDockerGameOptions(&cfg, engineImage, cfg.Engine.Version, mode, ddcPath, ddcZenPath, cfg.Engine.Backend)
	opts.CookOnly = true
	builder := dockerbuild.NewDockerGameBuilder(opts, r)

	_, err := builder.Build(ctx)

	if err != nil {
		return toolError(fmt.Sprintf("DDC warmup failed: %v\n%s", err, out.String()))
	}

	return resultOK(ddcWarmResult{
		Success: true,
		Message: "DDC warmup complete",
	})
}
