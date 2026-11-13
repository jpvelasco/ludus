package globals

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

func TestResolveDestroyTargetsRetainsFailuresAndOrder(t *testing.T) {
	cfg := config.Defaults()
	var calls []string
	SwapResolveTarget(t, func(_ context.Context, got *config.Config, name string) (deploy.Target, error) {
		if got != cfg {
			t.Fatal("resolver received a different configuration")
		}
		calls = append(calls, name)
		return nil, fmt.Errorf("unavailable %s", name)
	})
	targets := ResolveDestroyTargets(context.Background(), cfg)
	if !reflect.DeepEqual(calls, deploy.TargetNames()) || len(targets) != len(calls) {
		t.Fatalf("incomplete resolution: %v, %+v", calls, targets)
	}
	for i, target := range targets {
		if target.Name != calls[i] || target.Error == nil || target.Target != nil {
			t.Errorf("lost resolution result: %+v", target)
		}
	}
}

func TestPlanDestroyScopeAxesAndImmutableInputs(t *testing.T) {
	for _, tt := range []struct {
		name               string
		all, purge         bool
		targets, artifacts int
	}{
		{"active only", false, false, 1, 0},
		{"all targets", true, false, 5, 0},
		{"active purge", false, true, 1, 2},
		{"all and purge", true, true, 5, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkDestroyPlanScope(t, tt.all, tt.purge, tt.targets, tt.artifacts)
		})
	}
}

func checkDestroyPlanScope(t *testing.T, all, purge bool, targetCount, artifactCount int) {
	t.Helper()
	t.Chdir(t.TempDir())
	cfg := config.Defaults()
	cfg.Deploy.Target = "binary"
	cfg.AWS.Tags = map[string]string{"ManagedBy": "team", "Project": "example"}
	st := &state.State{Deploy: &state.DeployState{TargetName: "stack"}, Fleet: &state.FleetState{StackName: "recorded-stack"}}
	beforeCfg := cfg.Clone()
	beforeState, _ := json.Marshal(st)
	SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		t.Fatal("offline plan constructed an execution target")
		return nil, nil
	})
	plan, err := PlanDestroy(cfg, st, "preview-profile", "", all, purge)
	if err != nil {
		t.Fatal(err)
	}
	afterState, _ := json.Marshal(st)
	for name, ok := range map[string]bool{
		"target count":     len(plan.Targets) == targetCount,
		"purge count":      len(plan.Purge) == artifactCount,
		"profile":          plan.Profile == "preview-profile",
		"config unchanged": reflect.DeepEqual(beforeCfg, *cfg),
		"state unchanged":  string(beforeState) == string(afterState),
	} {
		if !ok {
			t.Errorf("%s failed: %+v", name, plan)
		}
	}
	data, err := json.Marshal(plan)
	if err != nil || len(data) == 0 || plan.Targets == nil || plan.Purge == nil {
		t.Fatalf("invalid plan encoding: %s, %v", data, err)
	}
}

func TestPlanDestroyUsesConfigOrOverrideRatherThanRecordedTarget(t *testing.T) {
	cfg := config.Defaults()
	cfg.Deploy.Target = ""
	st := &state.State{Deploy: &state.DeployState{TargetName: "stack"}}
	for _, tt := range []struct{ override, want string }{{"", "gamelift"}, {"binary", "binary"}} {
		plan, err := PlanDestroy(cfg, st, "", tt.override, false, false)
		if err != nil || plan.Targets[0].Target != tt.want {
			t.Fatalf("target = %+v, %v; want %s", plan, err, tt.want)
		}
	}
}

func TestPlanDestroyReportsLocalFailures(t *testing.T) {
	for _, tt := range []struct {
		name   string
		cfg    *config.Config
		st     *state.State
		target string
		all    bool
	}{
		{"missing config", nil, &state.State{}, "", false},
		{"missing snapshot", config.Defaults(), nil, "", false},
		{"unknown target", config.Defaults(), &state.State{}, "unknown", false},
		{"incomplete sweep", &config.Config{}, &state.State{}, "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := PlanDestroy(tt.cfg, tt.st, "", tt.target, tt.all, false); err == nil {
				t.Fatal("invalid local plan reported success")
			}
		})
	}
}
