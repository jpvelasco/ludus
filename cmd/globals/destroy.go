package globals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jpvelasco/ludus/internal/anywhere"
	"github.com/jpvelasco/ludus/internal/binary"
	"github.com/jpvelasco/ludus/internal/cleanup"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/ec2fleet"
	"github.com/jpvelasco/ludus/internal/gamelift"
	"github.com/jpvelasco/ludus/internal/stack"
	"github.com/jpvelasco/ludus/internal/state"
)

// PlanDestroy assembles a plan from one caller-loaded state snapshot. It does
// not resolve execution targets, AWS identity, runners, or build prerequisites.
func PlanDestroy(cfg *config.Config, st *state.State, profile, targetOverride string, allTargets, purge bool) (*deploy.DestroyPlan, error) {
	if cfg == nil || st == nil {
		return nil, fmt.Errorf("destroy preview requires configuration and a state snapshot")
	}
	target := targetOverride
	if target == "" {
		target = cfg.Deploy.Target
	}
	if target == "" {
		target = "gamelift"
	}
	names := []string{target}
	if allTargets {
		names = deploy.TargetNames()
	}
	plan := &deploy.DestroyPlan{
		Version: 1, Profile: profile, ConfiguredRegion: cfg.AWS.Region, ConfiguredAccountID: cfg.AWS.AccountID,
		Targets: []deploy.DestroyTargetPlan{}, Purge: []deploy.DestroyItem{},
	}
	var failures []error
	for _, name := range names {
		targetPlan, err := planDestroyTarget(cfg, st, name)
		if err != nil {
			failure := fmt.Errorf("planning %s destruction: %w", name, err)
			failures = append(failures, failure)
			plan.Warnings = append(plan.Warnings, failure.Error())
			continue
		}
		plan.Targets = append(plan.Targets, targetPlan)
	}
	if purge {
		plan.Purge = cleanup.PlanPurge(cfg)
	}
	return plan, errors.Join(failures...)
}

func planDestroyTarget(cfg *config.Config, st *state.State, name string) (deploy.DestroyTargetPlan, error) {
	switch name {
	case "gamelift":
		return gamelift.PlanDestroy(BuildGameLiftOptions(cfg, cfg.AWS.Region, ""), st)
	case "ec2":
		return ec2fleet.PlanDestroy(buildEC2Options(cfg, cfg.AWS.Region), st)
	case "stack":
		return stack.PlanDestroy(cfg.GameLift.FleetName, st), nil
	case "anywhere":
		return anywhere.PlanDestroy(st), nil
	case "binary":
		return binary.PlanDestroy(cfg.Deploy.OutputDir)
	default:
		return deploy.DestroyTargetPlan{}, fmt.Errorf("unknown deploy target %q", name)
	}
}

// DestroyTarget retains a target resolution result until the sweep executes.
type DestroyTarget struct {
	Name   string
	Target deploy.Target
	Error  error
}

// ResolveDestroyTargets resolves the whole sweep before any deletion can
// clear state needed by a later target's constructor.
func ResolveDestroyTargets(ctx context.Context, cfg *config.Config) []DestroyTarget {
	names := deploy.TargetNames()
	targets := make([]DestroyTarget, 0, len(names))
	for _, name := range names {
		target, err := ResolveTarget(ctx, cfg, name)
		targets = append(targets, DestroyTarget{Name: name, Target: target, Error: err})
	}
	return targets
}
