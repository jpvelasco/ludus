package pipeline

import (
	"os"
	"testing"

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
