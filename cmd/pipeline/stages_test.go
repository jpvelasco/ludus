package pipeline

import (
	"context"
	"testing"

	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/deploy"
)

// stubTarget is a minimal deploy.Target for testing buildStages.
type stubTarget struct {
	name string
	caps deploy.Capabilities
}

func (t *stubTarget) Name() string                      { return t.name }
func (t *stubTarget) Capabilities() deploy.Capabilities { return t.caps }
func (t *stubTarget) Deploy(_ context.Context, _ deploy.DeployInput) (*deploy.DeployResult, error) {
	return nil, nil
}
func (t *stubTarget) Status(_ context.Context) (*deploy.DeployStatus, error) { return nil, nil }
func (t *stubTarget) Destroy(_ context.Context) error                        { return nil }

func TestBuildStagesCount(t *testing.T) {
	p := &pipelineCtx{
		cfg: &config.Config{Game: config.GameConfig{ProjectName: "TestGame"}},
		target: &stubTarget{
			name: "binary",
			caps: deploy.Capabilities{},
		},
		arch: "amd64",
	}

	stages := buildStages(p)
	if len(stages) != 8 {
		t.Errorf("buildStages() returned %d stages, want 8", len(stages))
	}
}

func TestPipelineCtxWSL2Fields(t *testing.T) {
	p := &pipelineCtx{
		wslNative: true,
		wslDistro: "Ubuntu-24.04",
	}
	if !p.wslNative {
		t.Error("expected wslNative = true")
	}
	if p.wslDistro != "Ubuntu-24.04" {
		t.Errorf("wslDistro = %q, want %q", p.wslDistro, "Ubuntu-24.04")
	}
}
