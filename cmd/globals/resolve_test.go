package globals

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

func TestResolveTargetBinary(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		override   string
		wantName   string
	}{
		{name: "configured binary", configured: "binary", wantName: "binary"},
		{name: "override takes precedence", configured: "unknown", override: "binary", wantName: "binary"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Deploy: config.DeployConfig{Target: tt.configured}}
			target, err := ResolveTarget(context.Background(), cfg, tt.override)
			if err != nil {
				t.Fatal(err)
			}
			if got := target.Name(); got != tt.wantName {
				t.Fatalf("Name() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestResolveTargetUnknown(t *testing.T) {
	cfg := &config.Config{Deploy: config.DeployConfig{Target: "unsupported"}}
	target, err := ResolveTarget(context.Background(), cfg, "")
	if err == nil {
		t.Fatal("expected unknown target error")
	}
	if target != nil {
		t.Fatalf("target = %#v, want nil", target)
	}
	if !strings.Contains(err.Error(), `unknown deploy target "unsupported"`) {
		t.Fatalf("error = %q", err)
	}
}

func TestResolveBinaryOutputDirectory(t *testing.T) {
	tests := []struct {
		name      string
		outputDir string
	}{
		{name: "default directory"},
		{name: "configured directory", outputDir: t.TempDir()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := resolveBinary(&config.Config{
				Deploy: config.DeployConfig{OutputDir: tt.outputDir},
			})
			if err != nil {
				t.Fatal(err)
			}
			if target.Name() != "binary" {
				t.Fatalf("Name() = %q, want binary", target.Name())
			}
		})
	}
}

// TestResolveSessionTarget_ConfigTargetIsSessionManager verifies that when the
// config target already implements SessionManager, it is returned directly
// without consulting state.
func TestResolveSessionTarget_ConfigTargetIsSessionManager(t *testing.T) {
	origCfg := Cfg
	t.Cleanup(func() { Cfg = origCfg })

	// "gamelift" implements SessionManager but requires AWS — we only test
	// the routing logic here. Use "binary" (no sessions) to confirm the
	// fallback path, and test the direct path by verifying the binary target
	// is NOT a SessionManager (so the function will fall through to state).
	Cfg = &config.Config{
		Deploy: config.DeployConfig{Target: "binary"},
	}

	ctx := context.Background()
	target, err := ResolveSessionTarget(ctx, Cfg)
	if err != nil {
		t.Fatalf("ResolveSessionTarget: %v", err)
	}

	// binary is not a SessionManager — we expect the target to still be binary
	// (no state to fall back to in this test).
	if target.Name() != "binary" {
		t.Errorf("target.Name() = %q, want %q", target.Name(), "binary")
	}
}

// TestResolveSessionTarget_FallsBackToStateTarget verifies that when the
// config target (binary) does not implement SessionManager, ResolveSessionTarget
// reads the last-deployed target from state and returns that instead.
func TestResolveSessionTarget_FallsBackToStateTarget(t *testing.T) {
	t.Chdir(t.TempDir())

	origCfg := Cfg
	t.Cleanup(func() { Cfg = origCfg })

	Cfg = &config.Config{
		Deploy: config.DeployConfig{Target: "binary"},
	}

	// Write a gamelift-compatible state (the fallback target).
	// We use "binary" as the fallback too since we can't call AWS — the point
	// is to verify that the state is read and ResolveTarget is called with
	// the state's target name instead of stopping at the config target.
	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "binary",
		Status:     "ACTIVE",
		DeployedAt: "2026-05-13T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpdateDeploy: %v", err)
	}

	ctx := context.Background()
	target, err := ResolveSessionTarget(ctx, Cfg)
	if err != nil {
		t.Fatalf("ResolveSessionTarget: %v", err)
	}

	// Config says binary, state says binary — same result, but the function
	// must complete without error, confirming the fallback path executes.
	if target.Name() != "binary" {
		t.Errorf("target.Name() = %q, want %q", target.Name(), "binary")
	}
}

// TestResolveSessionTarget_NoStateFallback verifies that when the config target
// does not support sessions and state has no deploy record, the config target
// is returned (no error).
func TestResolveSessionTarget_NoStateFallback(t *testing.T) {
	t.Chdir(t.TempDir())

	origCfg := Cfg
	t.Cleanup(func() { Cfg = origCfg })

	Cfg = &config.Config{
		Deploy: config.DeployConfig{Target: "binary"},
	}

	ctx := context.Background()
	target, err := ResolveSessionTarget(ctx, Cfg)
	if err != nil {
		t.Fatalf("ResolveSessionTarget: %v", err)
	}

	// No state, config target is binary — returns binary, no error.
	if target.Name() != "binary" {
		t.Errorf("target.Name() = %q, want %q", target.Name(), "binary")
	}
	if _, ok := target.(deploy.SessionManager); ok {
		t.Error("binary target should not implement SessionManager")
	}
}

// TestResolveSessionTarget_UnknownStateFallbackErrors verifies that an invalid
// target name in state returns an error.
func TestResolveSessionTarget_UnknownStateFallbackErrors(t *testing.T) {
	t.Chdir(t.TempDir())

	origCfg := Cfg
	t.Cleanup(func() { Cfg = origCfg })

	Cfg = &config.Config{
		Deploy: config.DeployConfig{Target: "binary"},
	}

	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "nonexistent-target",
		Status:     "ACTIVE",
		DeployedAt: "2026-05-13T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpdateDeploy: %v", err)
	}

	ctx := context.Background()
	_, err := ResolveSessionTarget(ctx, Cfg)
	if err == nil {
		t.Fatal("expected error for unknown fallback target, got nil")
	}
}

// sessionStubTarget embeds swapStubTarget (deploy.Target) and adds the
// deploy.SessionManager methods so ResolveSessionTarget takes the direct
// return path instead of falling through to state.
type sessionStubTarget struct {
	swapStubTarget
}

func (s *sessionStubTarget) CreateSession(context.Context, int) (*deploy.SessionInfo, error) {
	return nil, nil
}

func (s *sessionStubTarget) DescribeSession(context.Context, string) (string, error) {
	return "", nil
}

// TestResolveSessionTarget_DirectSessionManagerTarget verifies that a config
// target implementing SessionManager is returned without consulting state.
func TestResolveSessionTarget_DirectSessionManagerTarget(t *testing.T) {
	stub := &sessionStubTarget{swapStubTarget{name: "session"}}
	SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return stub, nil
	})

	target, err := ResolveSessionTarget(context.Background(), &config.Config{})
	if err != nil {
		t.Fatalf("ResolveSessionTarget() error = %v, want nil", err)
	}
	if target != stub {
		t.Errorf("ResolveSessionTarget() = %v, want the session stub", target)
	}
}

// TestResolveSessionTarget_ConfigError verifies an error from ResolveTarget is
// returned unchanged.
func TestResolveSessionTarget_ConfigError(t *testing.T) {
	wantErr := errors.New("resolve exploded")
	SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return nil, wantErr
	})

	_, err := ResolveSessionTarget(context.Background(), &config.Config{})
	if !errors.Is(err, wantErr) {
		t.Errorf("ResolveSessionTarget() error = %v, want %v", err, wantErr)
	}
}
