package deploy

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/gamelift"
	"github.com/jpvelasco/ludus/internal/stack"
	"github.com/jpvelasco/ludus/internal/state"
)

func TestRunFleetDryRun(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg(), globals.WithDryRun(true))
	saveDeployFlags(t)
	if err := runFleet(newCommand(), nil); err != nil {
		t.Fatalf("runFleet() dry-run error = %v", err)
	}
}

func TestRunFleetDryRunWithInstanceTypeFlag(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg(), globals.WithDryRun(true))
	saveDeployFlags(t)
	instanceType = "c6i.xlarge"
	if err := runFleet(newCommand(), nil); err != nil {
		t.Fatalf("runFleet() dry-run error = %v", err)
	}
}

func TestRunFleetMakeDeployerError(t *testing.T) {
	cfg := testGameliftCfg()
	cfg.AWS.ECRRepository = ""
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	saveDeployFlags(t)
	if err := runFleet(newCommand(), nil); err == nil {
		t.Fatal("runFleet() expected makeDeployer error")
	}
}

func TestRunStackDryRun(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg(), globals.WithDryRun(true))
	saveDeployFlags(t)
	if err := runStack(newCommand(), nil); err != nil {
		t.Fatalf("runStack() dry-run error = %v", err)
	}
}

func TestRunStackFlagOverrides(t *testing.T) {
	cfg := testGameliftCfg()
	cfg.Game.Arch = "arm64"
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	saveDeployFlags(t)
	region = "eu-west-1"
	instanceType = "c6i.large"
	stackName = "custom-stack"

	var runErr error
	out := captureStdout(func() { runErr = runStack(newCommand(), nil) })
	if runErr != nil {
		t.Fatalf("runStack() error = %v", runErr)
	}
	if !strings.Contains(out, "Switching instance type") {
		t.Errorf("runStack() output missing switch note: %q", out)
	}
}

func TestRunStackApplyFlagsError(t *testing.T) {
	cfg := testGameliftCfg()
	cfg.AWS.ECRRepository = ""
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	saveDeployFlags(t)
	if err := runStack(newCommand(), nil); err == nil {
		t.Fatal("runStack() expected applyStackFlags error")
	}
}

func TestApplyStackFlagsResolveError(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg(), globals.WithDryRun(true))
	t.Setenv("AWS_SDK_LOAD_CONFIG", "1")
	t.Setenv("AWS_CONFIG_FILE", t.TempDir())
	saveDeployFlags(t)

	cfg := globals.Cfg.Clone()
	if _, _, _, _, err := applyStackFlags(context.Background(), &cfg); err == nil {
		t.Fatal("applyStackFlags() expected aws resolution error")
	}
}

func TestSaveStackState(t *testing.T) {
	t.Chdir(t.TempDir())
	saveStackState(&stack.StackResult{StackName: "ludus-fleet", StackID: "arn:stack", Status: "CREATE_COMPLETE", FleetID: "fleet-42"})

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	if s.Fleet == nil || s.Fleet.StackName != "ludus-fleet" || s.Fleet.FleetID != "fleet-42" {
		t.Errorf("fleet state = %+v, want stack ludus-fleet / fleet-42", s.Fleet)
	}
	if s.Deploy == nil || s.Deploy.TargetName != "stack" {
		t.Errorf("deploy state = %+v, want target stack", s.Deploy)
	}
}

func TestSaveStackStateWriteWarning(t *testing.T) {
	blockStateWrites(t)
	saveStackState(&stack.StackResult{StackName: "st", StackID: "si", Status: "ACTIVE", FleetID: "f1"})
}

func TestRecordFleetDeployStateWriteWarning(t *testing.T) {
	blockStateWrites(t)
	recordFleetDeployState(&gamelift.FleetStatus{FleetID: "fleet-x", Status: "ACTIVE"})
}
