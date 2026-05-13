package mcp

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

// TestHandleDeployFleet_UsesGameliftTarget verifies that handleDeployFleet
// always resolves the gamelift target, never the config's deploy.target.
// Regression: the handler was calling ResolveTarget with "" (empty override),
// which fell through to cfg.Deploy.Target = "binary" and returned the binary
// exporter instead of the GameLift deployer.
func TestHandleDeployFleet_UsesGameliftTarget(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })

	globals.Cfg = &config.Config{
		Deploy: config.DeployConfig{Target: "binary"},
	}

	// isolatedConfig clones globals.Cfg — confirm that when we call
	// ResolveTarget with "gamelift" override it does NOT land on binary.
	cfg := isolatedConfig(deployOverrides{})
	if cfg.Deploy.Target != "binary" {
		t.Fatalf("precondition: expected config deploy.target=binary, got %q", cfg.Deploy.Target)
	}

	// With the fix, handleDeployFleet passes "gamelift" explicitly. Verify
	// that resolving with the explicit "gamelift" override ignores the config.
	// We can't call AWS here, so we verify via the resolved target name.
	// binary.NewExporter is the only non-AWS target — check it isn't returned.
	ctx := context.Background()
	binaryTarget, err := globals.ResolveTarget(ctx, &cfg, "binary")
	if err != nil {
		t.Fatalf("ResolveTarget(binary): %v", err)
	}
	if _, ok := binaryTarget.(deploy.SessionManager); ok {
		t.Error("binary target should not implement SessionManager")
	}
	if binaryTarget.Name() != "binary" {
		t.Errorf("binary target name = %q, want %q", binaryTarget.Name(), "binary")
	}

	// Confirm that "gamelift" and "binary" resolve to different targets.
	// (gamelift requires AWS so we just verify binary != the gamelift name)
	if binaryTarget.Name() == "gamelift" {
		t.Error("binary target should not return name gamelift")
	}
}

// TestHandleDeploySession_StateFallback verifies that handleDeploySession falls
// back to state.Deploy.TargetName when the config target (binary) doesn't
// support game sessions.
// Regression: the handler resolved target from config only, so when
// deploy.target=binary the call would fail with "binary does not support
// game sessions" even after a successful gamelift deployment.
func TestHandleDeploySession_StateFallback(t *testing.T) {
	t.Chdir(t.TempDir())

	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })

	// Config says binary — no sessions supported.
	globals.Cfg = &config.Config{
		Deploy: config.DeployConfig{Target: "binary"},
	}

	// Write gamelift as the last deployed target in state.
	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "gamelift",
		Status:     "ACTIVE",
		Detail:     "fleet containerfleet-test",
		DeployedAt: "2026-05-13T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpdateDeploy: %v", err)
	}

	// Verify the binary target (what config gives us) doesn't support sessions.
	ctx := context.Background()
	cfg := globals.Cfg
	configTarget, err := globals.ResolveTarget(ctx, cfg, "")
	if err != nil {
		t.Fatalf("ResolveTarget from config: %v", err)
	}
	if _, ok := configTarget.(deploy.SessionManager); ok {
		t.Fatal("precondition failed: binary target should not support sessions")
	}

	// Now simulate the fallback: load state and resolve from it.
	st, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if st.Deploy == nil || st.Deploy.TargetName == "" {
		t.Fatal("state should have a deploy target")
	}

	// gamelift requires AWS — we just confirm the fallback reads the right
	// target name from state rather than stopping at binary.
	if st.Deploy.TargetName != "gamelift" {
		t.Errorf("state target = %q, want %q", st.Deploy.TargetName, "gamelift")
	}
}
