package dockerbuild

import (
	"fmt"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
)

// ResolveEngineImage determines the Docker image to use for builds.
// Precedence: config DockerImage > state EngineImage > constructed from config.
// When requireVersion is true, returns an error if the engine version cannot
// be detected (used by DDC warmup where "latest" is not meaningful).
func ResolveEngineImage(cfg *config.Config, requireVersion bool) (string, error) {
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
	version := cfg.Engine.Version
	if version == "" {
		if requireVersion {
			return "", fmt.Errorf("could not detect engine version (source_path=%q, version=%q); set engine.version or engine.docker_image in ludus.yaml",
				cfg.Engine.SourcePath, cfg.Engine.Version)
		}
		version = "latest"
	}
	return fmt.Sprintf("%s:%s", imageName, version), nil
}
