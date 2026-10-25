package pipeline

import (
	"os"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/game"
	"github.com/jpvelasco/ludus/internal/state"
)

// blockStateWrites replaces the .ludus directory with a file so every
// state.Save fails at MkdirAll, forcing the save*State warning branches.
func blockStateWrites(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	orig := state.ActiveProfile()
	t.Cleanup(func() { state.SetProfile(orig) })
	state.SetProfile("")
	if err := os.WriteFile(".ludus", []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestPipelineSaveStateSkippedUnderDryRun pins #733: a dry-run pipeline run
// reports successful builds without writing any state — the planned outputs
// were never built.
func TestPipelineSaveStateSkippedUnderDryRun(t *testing.T) {
	tests := []struct {
		name string
		save func(p *pipelineCtx)
	}{
		{
			name: "client state",
			save: func(p *pipelineCtx) {
				p.saveClientState(&game.ClientBuildResult{
					ClientBinary: "/planned/client", OutputDir: "/planned", Platform: "Linux",
				})
			},
		},
		{
			name: "engine image state",
			save: func(p *pipelineCtx) {
				p.saveEngineImageState("ludus-engine:planned")
			},
		},
		{
			name: "wsl2 engine state",
			save: func(p *pipelineCtx) {
				p.saveWSL2EngineState("/planned/wsl/engine", "/planned/wsl/ddc")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runPipelineSaveDryRunCase(t, tt.save)
		})
	}
}

func runPipelineSaveDryRunCase(t *testing.T, save func(p *pipelineCtx)) {
	t.Helper()
	t.Chdir(t.TempDir())
	origProfile := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(origProfile) })
	globals.SetGlobals(t, &config.Config{}, globals.WithDryRun(true))

	save(&pipelineCtx{wslNative: true})

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	if s.Client != nil {
		t.Errorf("dry-run pipeline run wrote client state: %+v", s.Client)
	}
	if s.EngineImage != nil {
		t.Errorf("dry-run pipeline run wrote engine image state: %+v", s.EngineImage)
	}
	if s.WSL2Engine != nil {
		t.Errorf("dry-run pipeline run wrote wsl2 engine state: %+v", s.WSL2Engine)
	}
}

func TestSaveDeployState_WriteFails(t *testing.T) {
	blockStateWrites(t)
	p := &pipelineCtx{}
	p.saveDeployState(&deploy.DeployResult{
		TargetName: "binary",
		Status:     "exported",
		Detail:     "/tmp/out",
	})
}

func TestSaveEngineImageState_WriteFails(t *testing.T) {
	blockStateWrites(t)
	p := &pipelineCtx{}
	p.saveEngineImageState("v26")
}

func TestSaveClientState_WriteFails(t *testing.T) {
	blockStateWrites(t)
	p := &pipelineCtx{}
	p.saveClientState(&game.ClientBuildResult{
		ClientBinary: "/tmp/client",
		OutputDir:    "/tmp/out",
		Platform:     "windows",
	})
}
