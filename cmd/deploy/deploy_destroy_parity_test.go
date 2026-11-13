package deploy

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/stack"
	"github.com/jpvelasco/ludus/internal/state"
)

func TestDestroySweepResolvesRecordedStackBeforeStateClear(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, config.Defaults())
	if err := state.Save(&state.State{Deploy: &state.DeployState{TargetName: "stack"}, Fleet: &state.FleetState{StackName: "recorded-custom"}}); err != nil {
		t.Fatal(err)
	}
	resolved, stackName := 0, ""
	globals.SwapResolveTarget(t, func(_ context.Context, _ *config.Config, name string) (deploy.Target, error) {
		resolved++
		target := &fakeTarget{name: name}
		if name == "gamelift" {
			target.onDestroy = func() error {
				if resolved != 5 {
					t.Errorf("only %d targets resolved before mutation", resolved)
				}
				return state.ClearFleet()
			}
		}
		if name == "stack" {
			st, err := state.Load()
			if err != nil {
				return nil, err
			}
			stackName = stack.RecordedStackName(st)
		}
		return target, nil
	})
	if err := destroyAllTargets(context.Background(), globals.Cfg); err != nil {
		t.Fatal(err)
	}
	if stackName != "recorded-custom" {
		t.Fatalf("sweep lost recorded stack name: %q", stackName)
	}
}

func TestDestroyFlagsApplyToSingleAndSweep(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "sweep"}[all], func(t *testing.T) {
			globals.SetGlobals(t, config.Defaults())
			saveDeployFlags(t)
			destroyAllTgts, destroyPurge = all, false
			fleetName, instanceType, region = "flag-fleet", "c6i.xlarge", "us-west-2"
			globals.SwapResolveTarget(t, func(_ context.Context, cfg *config.Config, name string) (deploy.Target, error) {
				if cfg.GameLift.FleetName != "flag-fleet" || cfg.GameLift.InstanceType != "c6i.xlarge" || cfg.AWS.Region != "us-west-2" {
					t.Errorf("destroy flags lost: %+v", cfg)
				}
				return &fakeTarget{name: name}, nil
			})
			if err := runDestroy(newCommand(), nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}
