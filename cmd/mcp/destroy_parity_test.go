package mcp

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
)

func TestMCPDestroySweepResolvesAllTargetsBeforeMutation(t *testing.T) {
	resolved := 0
	globals.SwapResolveTarget(t, func(_ context.Context, _ *config.Config, name string) (deploy.Target, error) {
		resolved++
		return &testDeployTarget{name: name, onDestroy: func() error {
			if resolved != 5 {
				t.Errorf("only %d targets resolved before mutation", resolved)
			}
			return nil
		}}, nil
	})
	if err := destroyAllTargets(context.Background(), config.Defaults(), newToolOutput()); err != nil {
		t.Fatal(err)
	}
}
