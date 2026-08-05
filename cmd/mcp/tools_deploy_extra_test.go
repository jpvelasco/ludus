package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

// TestHandleDeployFleetResolveError covers the ResolveTarget failure branch
// (tools_deploy.go:176-179).
func TestHandleDeployFleetResolveError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return nil, errors.New("resolve exploded")
	})

	result, _, err := handleDeployFleet(context.Background(), nil, deployFleetInput{})
	if err != nil {
		t.Fatalf("handleDeployFleet() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "could not resolve deploy target") {
		t.Errorf("result = %q, want resolution failure", text)
	}
}

// TestHandleDeployFleetDeployError covers the target.Deploy failure branch
// (tools_deploy.go:208-211).
func TestHandleDeployFleetDeployError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{
		AWS:       config.AWSConfig{ECRRepository: "ludus-server"},
		Container: config.ContainerConfig{Tag: "5.7.3"},
	}, globals.WithDryRun(true))

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "gamelift", err: errors.New("fleet deploy failed")}, nil
	})

	result, _, err := handleDeployFleet(context.Background(), nil, deployFleetInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleDeployFleet() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "fleet deployment failed") {
		t.Errorf("result = %q, want fleet deployment failure", text)
	}
}

// TestHandleDeployStackAutoSwitchAndEnvError covers the pricing.AutoSwitch branch
// (tools_deploy.go:234-236) and the AWS env resolution failure (244-246). The
// mismatched arch/instance type triggers the switch before the env resolve fails.
func TestHandleDeployStackAutoSwitchAndEnvError(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AWS_PROFILE", "ludus-does-not-exist-anywhere")
	t.Setenv("AWS_SDK_LOAD_CONFIG", "1")

	cfg := &config.Config{
		Game:     config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject", Arch: "arm64"},
		GameLift: config.GameLiftConfig{FleetName: "testfleet", InstanceType: "c5.large"},
	}
	globals.SetGlobals(t, cfg)

	result, _, err := handleDeployStack(context.Background(), nil, deployStackInput{})
	if err != nil {
		t.Fatalf("handleDeployStack() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "could not resolve AWS environment") {
		t.Errorf("result = %q, want AWS environment failure", text)
	}
}

// TestHandleDeployAnywhereResolveError covers the anywhere ResolveTarget branch
// (tools_deploy.go:297-299).
func TestHandleDeployAnywhereResolveError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return nil, errors.New("resolve exploded")
	})

	result, _, err := handleDeployAnywhere(context.Background(), nil, deployAnywhereInput{})
	if err != nil {
		t.Fatalf("handleDeployAnywhere() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "could not resolve anywhere target") {
		t.Errorf("result = %q, want anywhere resolution failure", text)
	}
}

// TestHandleDeployAnywhereDeployError covers the anywhere Deploy failure branch
// (tools_deploy.go:317-320).
func TestHandleDeployAnywhereDeployError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{}, globals.WithDryRun(true))

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "anywhere", err: errors.New("anywhere deploy failed")}, nil
	})

	result, _, err := handleDeployAnywhere(context.Background(), nil, deployAnywhereInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleDeployAnywhere() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "anywhere deployment failed") {
		t.Errorf("result = %q, want anywhere deployment failure", text)
	}
}

// TestHandleDeployEC2ResolveError covers the ec2 ResolveTarget branch
// (tools_deploy.go:343-345).
func TestHandleDeployEC2ResolveError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return nil, errors.New("resolve exploded")
	})

	result, _, err := handleDeployEC2(context.Background(), nil, deployEC2Input{})
	if err != nil {
		t.Fatalf("handleDeployEC2() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "could not resolve ec2 target") {
		t.Errorf("result = %q, want ec2 resolution failure", text)
	}
}

// TestHandleDeployEC2DeployError covers the ec2 Deploy failure branch
// (tools_deploy.go:363-366).
func TestHandleDeployEC2DeployError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{}, globals.WithDryRun(true))

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "ec2", err: errors.New("ec2 deploy failed")}, nil
	})

	result, _, err := handleDeployEC2(context.Background(), nil, deployEC2Input{DryRun: true})
	if err != nil {
		t.Fatalf("handleDeployEC2() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "EC2 fleet deployment failed") {
		t.Errorf("result = %q, want ec2 deployment failure", text)
	}
}

// TestHandleDeployEC2ReadsStateFleet covers the state EC2Fleet read after a
// successful deploy (tools_deploy.go:374-378).
func TestHandleDeployEC2ReadsStateFleet(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{
		Game: config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject", Arch: "amd64"},
	}, globals.WithDryRun(true))

	if err := state.UpdateEC2Fleet(&state.EC2FleetState{FleetID: "fleet-ec2-123", BuildID: "build-ec2-456"}); err != nil {
		t.Fatalf("state.UpdateEC2Fleet: %v", err)
	}

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "ec2", result: &deploy.DeployResult{TargetName: "ec2", Status: "CREATED"}}, nil
	})

	result, _, err := handleDeployEC2(context.Background(), nil, deployEC2Input{DryRun: true})
	if err != nil {
		t.Fatalf("handleDeployEC2() error = %v", err)
	}
	text := toolResultText(t, result)
	if !strings.Contains(text, "fleet-ec2-123") || !strings.Contains(text, "build-ec2-456") {
		t.Errorf("result = %q, want fleet and build IDs from state", text)
	}
}

// TestHandleDeploySessionResolveError covers the ResolveSessionTarget failure
// branch (tools_deploy.go:389-391).
func TestHandleDeploySessionResolveError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{Deploy: config.DeployConfig{Target: "gamelift"}})

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return nil, errors.New("resolve exploded")
	})

	result, _, err := handleDeploySession(context.Background(), nil, deploySessionInput{})
	if err != nil {
		t.Fatalf("handleDeploySession() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "could not resolve deploy target") {
		t.Errorf("result = %q, want session resolution failure", text)
	}
}

// TestRunDestroyForMCPDestroyError covers the single-target Destroy failure
// branch (tools_deploy.go:456-458).
func TestRunDestroyForMCPDestroyError(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{}

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "gamelift", err: errors.New("destroy exploded")}, nil
	})

	err := runDestroyForMCP(context.Background(), cfg, deployDestroyInput{Target: "gamelift"})
	if err == nil || !strings.Contains(err.Error(), "destroy exploded") {
		t.Fatalf("runDestroyForMCP() error = %v, want destroy failure", err)
	}
}

// TestDestroyAllTargetsContinuesOnDestroyError covers destroyAllTargets
// continuing past a Destroy error (tools_deploy.go:475-477).
func TestDestroyAllTargetsContinuesOnDestroyError(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{}

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "target", err: errors.New("destroy exploded")}, nil
	})

	err := runDestroyForMCP(context.Background(), cfg, deployDestroyInput{AllTargets: true})
	if err != nil {
		t.Fatalf("runDestroyForMCP(all) error = %v, want nil", err)
	}
}
