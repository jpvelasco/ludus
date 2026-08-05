package deploy

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
)

func TestRunSession(t *testing.T) {
	t.Run("success with explicit target", func(t *testing.T) {
		globals.SetGlobals(t, sessionCfg())
		saveDeployFlags(t)
		targetFlag = "gamelift"
		swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
			return &fakeTarget{name: "gamelift"}, nil
		})
		if err := runSession(newCommand(), nil); err != nil {
			t.Fatalf("runSession() error = %v", err)
		}
	})

	t.Run("success via session target fallback", func(t *testing.T) {
		globals.SetGlobals(t, sessionCfg())
		saveDeployFlags(t)
		targetFlag = ""
		swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
			return &fakeTarget{name: "gamelift"}, nil
		})
		if err := runSession(newCommand(), nil); err != nil {
			t.Fatalf("runSession() error = %v", err)
		}
	})

	t.Run("target without session support errors", func(t *testing.T) {
		globals.SetGlobals(t, sessionCfg())
		saveDeployFlags(t)
		targetFlag = "binary"
		swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
			return &nonSessionTarget{name: "binary"}, nil
		})

		err := runSession(newCommand(), nil)
		if err == nil {
			t.Fatal("runSession() expected error for non-session target")
		}
		if !strings.Contains(err.Error(), "does not support game sessions") {
			t.Errorf("error = %v, want does not support game sessions", err)
		}
	})

	t.Run("resolve failure propagates", func(t *testing.T) {
		globals.SetGlobals(t, sessionCfg())
		saveDeployFlags(t)
		targetFlag = "gamelift"
		swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
			return nil, fmt.Errorf("resolve boom")
		})
		if err := runSession(newCommand(), nil); err == nil {
			t.Fatal("runSession() expected resolve error")
		}
	})

	t.Run("session creation error propagates", func(t *testing.T) {
		globals.SetGlobals(t, sessionCfg())
		saveDeployFlags(t)
		targetFlag = "gamelift"
		swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
			return &fakeTarget{sessionErr: fmt.Errorf("create boom")}, nil
		})
		if err := runSession(newCommand(), nil); err == nil {
			t.Fatal("runSession() expected session creation error")
		}
	})
}

func sessionCfg() *config.Config {
	return &config.Config{Game: config.GameConfig{Arch: "amd64"}}
}
