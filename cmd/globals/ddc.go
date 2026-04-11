package globals

import (
	"fmt"

	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/ddc"
	"github.com/devrecon/ludus/internal/state"
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
// Config localPath takes precedence over the default path (~/.ludus/ddc).
// Validates that the path is absolute (relative paths break Docker volume mounts).
func ResolveDDCPath() (string, error) {
	if Cfg != nil && Cfg.DDC.LocalPath != "" {
		return ddc.ResolvePath(Cfg.DDC.LocalPath)
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

// ResolveEngineImage determines the Docker image to use for game builds.
// Precedence: config DockerImage > state EngineImage > constructed from config.
func ResolveEngineImage(cfg *config.Config) (string, error) {
	if cfg.Engine.DockerImage != "" {
		return cfg.Engine.DockerImage, nil
	}

	s, err := state.Load()
	if err == nil && s.EngineImage != nil && s.EngineImage.ImageTag != "" {
		return s.EngineImage.ImageTag, nil
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
	return fmt.Sprintf("%s:%s", imageName, tag), nil
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
	version, source := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
	if version == "" {
		return "", fmt.Errorf("could not detect engine version for DDC warmup (source_path=%q, version=%q, detection=%q); set engine.version or engine.docker_image in ludus.yaml",
			cfg.Engine.SourcePath, cfg.Engine.Version, source)
	}
	return fmt.Sprintf("%s:%s", imageName, version), nil
}
