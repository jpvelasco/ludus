package status

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
	"github.com/jpvelasco/ludus/internal/runner"
)

// CheckEngineBuild checks the artifact produced by the configured backend.
func CheckEngineBuild(ctx context.Context, cfg *config.Config) StageStatus {
	if !dockerbuild.IsContainerBackend(cfg.Engine.Backend) {
		return checkNativeEngineBuild(cfg.Engine.SourcePath)
	}
	s := StageStatus{Name: "Engine Build", Status: "ok"}
	if cfg.Engine.DockerImage != "" {
		s.Detail = "configured prebuilt image: " + cfg.Engine.DockerImage
		return s
	}
	image, err := dockerbuild.ResolveEngineImage(cfg, false)
	if err != nil {
		s.Status, s.Detail = "unknown", err.Error()
		return s
	}
	cli := cfg.Engine.Backend
	if _, err := exec.LookPath(cli); err != nil {
		s.Status, s.Detail = "unknown", cli+" not available; engine image: "+image
		return s
	}
	r := runner.NewRunner(false, false)
	r.Stdout, r.Stderr = io.Discard, io.Discard
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := r.RunQuietErr(ctx, cli, "image", "inspect", image); err != nil {
		s.Status = "fail"
		s.Detail = fmt.Sprintf("engine image %s not found or unavailable via %s: %v", image, cli, err)
		return s
	}
	s.Detail = "engine image found: " + image
	return s
}
