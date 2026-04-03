package globals

import (
	"fmt"

	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/ddc"
	"github.com/devrecon/ludus/internal/toolchain"
)

// ResolveDDCMode returns the effective DDC mode.
// CLI flag (DDCMode) takes precedence over config (Cfg.DDC.Mode).
// Returns an error for invalid mode values.
func ResolveDDCMode() (string, error) {
	var mode string
	if DDCMode != "" {
		mode = DDCMode
	} else if Cfg != nil && Cfg.DDC.Mode != "" {
		mode = Cfg.DDC.Mode
	}
	return ddc.ValidateDDCMode(mode)
}

// ResolveDDCPath returns the effective DDC host path.
// Config local_path takes precedence over the default path (~/.ludus/ddc).
func ResolveDDCPath() (string, error) {
	if Cfg != nil && Cfg.DDC.LocalPath != "" {
		return Cfg.DDC.LocalPath, nil
	}
	return ddc.DefaultPath()
}

// ResolveDDC returns the effective DDC mode and path.
// When mode is "none", path is returned empty (DDC is disabled).
func ResolveDDC() (mode, path string, err error) {
	mode, err = ResolveDDCMode()
	if err != nil {
		return "", "", err
	}
	if mode == "none" {
		return mode, "", nil
	}
	path, err = ResolveDDCPath()
	if err != nil {
		return "", "", fmt.Errorf("resolving DDC path: %w", err)
	}
	return mode, path, nil
}

// ResolveWarmupEngineImage determines the Docker image for DDC warmup.
// Returns an error if the engine version cannot be determined.
func ResolveWarmupEngineImage(cfg *config.Config) (string, error) {
	if cfg.Engine.DockerImage != "" {
		return cfg.Engine.DockerImage, nil
	}
	imageName := cfg.Engine.DockerImageName
	if imageName == "" {
		imageName = "ludus-engine"
	}
	version, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
	if version == "" {
		return "", fmt.Errorf("could not detect engine version for DDC warmup; set engine.version or engine.docker_image in ludus.yaml")
	}
	return fmt.Sprintf("%s:%s", imageName, version), nil
}
