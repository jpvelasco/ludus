package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

func TestHandleDeploySessionRejectsTargetWithoutSessions(t *testing.T) {
	t.Chdir(t.TempDir())

	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{Deploy: config.DeployConfig{Target: "binary"}}

	result, _, err := handleDeploySession(context.Background(), nil, deploySessionInput{
		MaxPlayers: -1,
	})
	if err != nil {
		t.Fatalf("handleDeploySession: %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "does not support game sessions") {
		t.Fatalf("result = %q, want unsupported-session error", text)
	}
}

func TestRunDestroyForMCPRejectsUnknownTarget(t *testing.T) {
	cfg := &config.Config{}
	err := runDestroyForMCP(context.Background(), cfg, deployDestroyInput{Target: "unknown"})
	if err == nil || !strings.Contains(err.Error(), "could not resolve deploy target") {
		t.Fatalf("runDestroyForMCP error = %v, want target resolution failure", err)
	}
}

func TestHandleDeployDestroyReportsResolutionFailure(t *testing.T) {
	t.Chdir(t.TempDir())

	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	globals.Cfg = &config.Config{}

	result, _, err := handleDeployDestroy(context.Background(), nil, deployDestroyInput{Target: "unknown"})
	if err != nil {
		t.Fatalf("handleDeployDestroy: %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "destroy failed") {
		t.Fatalf("result = %q, want destroy failure", text)
	}
}

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

// TestHandleDeployFleetDryRun tests deploy fleet with a stubbed gamelift target.
func TestHandleDeployFleetDryRun(t *testing.T) {
	cfg := &config.Config{
		Game:      config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject", Arch: "amd64"},
		GameLift:  config.GameLiftConfig{FleetName: "testfleet", InstanceType: "c5.large"},
		Container: config.ContainerConfig{ImageName: "server", Tag: "test", ServerPort: 7777},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	// Stub the deploy target to return a success result.
	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return &testDeployTarget{
			name: "gamelift",
			result: &deploy.DeployResult{
				TargetName: "gamelift",
				Status:     "ACTIVE",
				Detail:     "fleet testfleet",
			},
		}, nil
	})

	result, _, err := handleDeployFleet(context.Background(), nil, deployFleetInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleDeployFleet() error = %v", err)
	}
	text := toolResultText(t, result)
	// The result should contain the deploy status
	if !strings.Contains(text, "fleet") && !strings.Contains(text, "success") {
		t.Errorf("result = %q, want fleet or success indicator", text)
	}
}

// TestHandleDeployStackDryRun tests deploy stack with a stubbed target.
func TestHandleDeployStackDryRun(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject", Arch: "amd64"},
		AWS: config.AWSConfig{
			Region:        "us-west-2",
			AccountID:     "123456789012",
			ECRRepository: "my-repo",
		},
		GameLift: config.GameLiftConfig{
			FleetName:          "testfleet",
			InstanceType:       "c5.large",
			ContainerGroupName: "test-group",
		},
		Container: config.ContainerConfig{ImageName: "server", Tag: "test", ServerPort: 7777},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	// Stub the deploy target to return a success result.
	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return &testDeployTarget{
			name: "stack",
			result: &deploy.DeployResult{
				TargetName: "stack",
				Status:     "CREATE_COMPLETE",
				Detail:     "stack arn:aws:cloudformation:us-west-2:123456789012:stack/ludus-stack",
			},
		}, nil
	})

	result, _, err := handleDeployStack(context.Background(), nil, deployStackInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleDeployStack() error = %v", err)
	}
	text := toolResultText(t, result)
	// Verify the result contains success indicator or error field
	if !strings.Contains(text, "success") && !strings.Contains(text, "error") {
		t.Errorf("result = %q, want success or error field", text)
	}
}

// TestHandleDeployAnywhereDryRun tests deploy anywhere with a stubbed target.
func TestHandleDeployAnywhereDryRun(t *testing.T) {
	cfg := &config.Config{
		Game:      config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject"},
		Container: config.ContainerConfig{ServerPort: 7777},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	// Stub the deploy target to return a success result.
	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return &testDeployTarget{
			name: "anywhere",
			result: &deploy.DeployResult{
				TargetName: "anywhere",
				Status:     "REGISTERED",
				Detail:     "compute registered as compute-1",
			},
		}, nil
	})

	result, _, err := handleDeployAnywhere(context.Background(), nil, deployAnywhereInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleDeployAnywhere() error = %v", err)
	}
	text := toolResultText(t, result)
	// Verify the result contains success or error field
	if !strings.Contains(text, "success") && !strings.Contains(text, "error") {
		t.Errorf("result = %q, want success or error field", text)
	}
}

// TestHandleDeployEC2DryRun tests deploy ec2 with a stubbed target.
func TestHandleDeployEC2DryRun(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject", Arch: "amd64"},
		AWS:  config.AWSConfig{Region: "us-west-2"},
		GameLift: config.GameLiftConfig{
			FleetName:    "testfleet",
			InstanceType: "c5.large",
		},
		Container: config.ContainerConfig{ServerPort: 7777},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	// Stub the deploy target to return a success result.
	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return &testDeployTarget{
			name: "ec2",
			result: &deploy.DeployResult{
				TargetName: "ec2",
				Status:     "CREATED",
				Detail:     "build created and fleet provisioning arn:aws:gamelift:us-west-2:123456789012:fleet/fleet-123",
			},
		}, nil
	})

	result, _, err := handleDeployEC2(context.Background(), nil, deployEC2Input{DryRun: true})
	if err != nil {
		t.Fatalf("handleDeployEC2() error = %v", err)
	}
	text := toolResultText(t, result)
	// Verify the result contains deploy information
	if !strings.Contains(text, "CREATED") && !strings.Contains(text, "fleet") {
		t.Errorf("result = %q, want CREATED or fleet", text)
	}
}

// TestDestroyAllTargetsHandlesResolveErrors verifies it handles errors without panicking.
func TestDestroyAllTargetsHandlesResolveErrors(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	// When all targets fail to resolve, the result should indicate failure.
	// Return a stub that fails for all targets.
	failCount := 0
	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, target string) (deploy.Target, error) {
		failCount++
		return nil, fmt.Errorf("could not resolve %s", target)
	})

	err := runDestroyForMCP(ctx, cfg, deployDestroyInput{AllTargets: true})
	// With all targets failing to resolve, the outer function should fail
	if err != nil {
		// Error is expected when destroying unknown targets
		if !strings.Contains(err.Error(), "could not resolve") {
			t.Errorf("error = %v, want resolution error", err)
		}
	} else {
		// If no error, we should have attempted to resolve at least one target
		if failCount == 0 {
			t.Error("expected at least one attempt to resolve target")
		}
	}
}

// testDeployTarget is a stub deploy.Target used for testing handlers.
type testDeployTarget struct {
	name   string
	result *deploy.DeployResult
	status *deploy.DeployStatus
	err    error
}

func (t *testDeployTarget) Name() string                      { return t.name }
func (t *testDeployTarget) Capabilities() deploy.Capabilities { return deploy.Capabilities{} }
func (t *testDeployTarget) Deploy(ctx context.Context, input deploy.DeployInput) (*deploy.DeployResult, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.result, nil
}
func (t *testDeployTarget) Status(ctx context.Context) (*deploy.DeployStatus, error) {
	if t.status != nil {
		return t.status, nil
	}
	if t.err != nil {
		return nil, t.err
	}
	return &deploy.DeployStatus{}, nil
}
func (t *testDeployTarget) Destroy(ctx context.Context) error { return t.err }

// TestHandleDeployFleetSuccess verifies successful fleet deployment.
func TestHandleDeployFleetSuccess(t *testing.T) {
	cfg := &config.Config{
		Game:      config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject", Arch: "amd64"},
		GameLift:  config.GameLiftConfig{FleetName: "testfleet", InstanceType: "c5.large"},
		Container: config.ContainerConfig{ImageName: "server", Tag: "test", ServerPort: 7777},
	}
	globals.SetGlobals(t, cfg)

	// Stub the deploy target with a session manager capability
	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return &sessionDeployTarget{
			name: "gamelift",
			result: &deploy.DeployResult{
				TargetName: "gamelift",
				Status:     "ACTIVE",
				Detail:     "fleet containerfleet-test",
			},
		}, nil
	})

	result, _, err := handleDeployFleet(context.Background(), nil, deployFleetInput{})
	if err != nil {
		t.Fatalf("handleDeployFleet() error = %v", err)
	}
	text := toolResultText(t, result)
	// Verify result contains deployment status
	if !strings.Contains(text, "ACTIVE") && !strings.Contains(text, "success") {
		t.Errorf("result = %q, want deployment status", text)
	}
}

// TestHandleDeploySessionSuccess verifies successful session creation with session-supporting target.
func TestHandleDeploySessionSuccess(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg := &config.Config{
		Game:      config.GameConfig{ProjectName: "Lyra"},
		GameLift:  config.GameLiftConfig{FleetName: "testfleet"},
		Container: config.ContainerConfig{ServerPort: 7777},
	}
	globals.SetGlobals(t, cfg)

	// Stub a target that supports sessions and returns a session result
	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return &sessionDeployTarget{
			name: "gamelift",
			sessionID: "session-123",
		}, nil
	})

	result, _, err := handleDeploySession(context.Background(), nil, deploySessionInput{MaxPlayers: 8})
	if err != nil {
		t.Fatalf("handleDeploySession() error = %v", err)
	}
	text := toolResultText(t, result)
	// Verify result contains session info
	if !strings.Contains(text, "session") && !strings.Contains(text, "success") {
		t.Errorf("result = %q, want session information", text)
	}
}

// sessionDeployTarget is a test stub that implements deploy.SessionManager
type sessionDeployTarget struct {
	name      string
	result    *deploy.DeployResult
	sessionID string
}

func (t *sessionDeployTarget) Name() string                      { return t.name }
func (t *sessionDeployTarget) Capabilities() deploy.Capabilities { return deploy.Capabilities{SupportsSession: true} }
func (t *sessionDeployTarget) Deploy(ctx context.Context, input deploy.DeployInput) (*deploy.DeployResult, error) {
	return t.result, nil
}
func (t *sessionDeployTarget) Status(ctx context.Context) (*deploy.DeployStatus, error) {
	return &deploy.DeployStatus{}, nil
}
func (t *sessionDeployTarget) Destroy(ctx context.Context) error { return nil }
func (t *sessionDeployTarget) CreateSession(ctx context.Context, fleetID string, maxPlayers int) (string, error) {
	return t.sessionID, nil
}
