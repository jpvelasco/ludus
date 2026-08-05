package deploy

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

func TestDryRun(t *testing.T) {
	tests := []struct {
		name string
		dry  bool
		want bool
	}{
		{"enabled short-circuits", true, true},
		{"disabled proceeds", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals.SetGlobals(t, &config.Config{}, globals.WithDryRun(tt.dry))
			if got := dryRun("dry-run message"); got != tt.want {
				t.Errorf("dryRun() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrintPricingHints(t *testing.T) {
	tests := []struct {
		name    string
		it      string
		arch    string
		wantEst bool
		wantTip bool
	}{
		{"known x86 instance prints estimate and tip", "c6i.large", "amd64", true, true},
		{"unknown instance prints nothing", "bogus-type", "arm64", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(func() { printPricingHints(tt.it, tt.arch) })
			if got := strings.Contains(out, "Estimated cost"); got != tt.wantEst {
				t.Errorf("estimate printed = %v, want %v (output: %q)", got, tt.wantEst, out)
			}
			if got := strings.Contains(out, "Tip:"); got != tt.wantTip {
				t.Errorf("tip printed = %v, want %v (output: %q)", got, tt.wantTip, out)
			}
		})
	}
}

func TestPrintNextStep(t *testing.T) {
	tests := []struct {
		name        string
		withSession bool
		want        string
	}{
		{"session created points to connect", true, "ludus connect"},
		{"no session points to deploy session", false, "ludus deploy session"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveDeployFlags(t)
			withSession = tt.withSession
			out := captureStdout(printNextStep)
			if !strings.Contains(out, tt.want) {
				t.Errorf("printNextStep() output %q missing %q", out, tt.want)
			}
		})
	}
}

func TestResolveTargetFlagOverrides(t *testing.T) {
	globals.SetGlobals(t, &config.Config{
		AWS:      config.AWSConfig{Region: "us-east-1"},
		GameLift: config.GameLiftConfig{InstanceType: "c6i.large", FleetName: "ludus-fleet"},
	})

	tests := []struct {
		name                                        string
		region, instanceType, fleetName, targetFlag string
		wantRegion, wantIT, wantFleet, wantOverride string
	}{
		{"no flags fall back to config defaults", "", "", "", "", "us-east-1", "c6i.large", "ludus-fleet", ""},
		{"flags override config and target", "eu-west-1", "c7g.large", "override-fleet", "stack", "eu-west-1", "c7g.large", "override-fleet", "stack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveDeployFlags(t)
			region, instanceType, fleetName, targetFlag = tt.region, tt.instanceType, tt.fleetName, tt.targetFlag

			var gotCfg *config.Config
			var gotOverride string
			swapTargetFactory(t, func(_ context.Context, cfg *config.Config, override string) (deploy.Target, error) {
				gotCfg = cfg
				gotOverride = override
				return &fakeTarget{name: "gamelift"}, nil
			})

			target, err := resolveTarget(newCommand())
			if err != nil {
				t.Fatalf("resolveTarget() error = %v", err)
			}
			if target == nil {
				t.Fatal("resolveTarget() returned nil target")
			}
			checkResolvedOverrides(t, gotCfg, gotOverride, tt.wantRegion, tt.wantIT, tt.wantFleet, tt.wantOverride)
		})
	}
}

func checkResolvedOverrides(t *testing.T, cfg *config.Config, gotOverride, wantRegion, wantIT, wantFleet, wantOverride string) {
	t.Helper()
	if cfg.AWS.Region != wantRegion {
		t.Errorf("resolved Region = %q, want %q", cfg.AWS.Region, wantRegion)
	}
	if cfg.GameLift.InstanceType != wantIT {
		t.Errorf("resolved InstanceType = %q, want %q", cfg.GameLift.InstanceType, wantIT)
	}
	if cfg.GameLift.FleetName != wantFleet {
		t.Errorf("resolved FleetName = %q, want %q", cfg.GameLift.FleetName, wantFleet)
	}
	if gotOverride != wantOverride {
		t.Errorf("resolved target override = %q, want %q", gotOverride, wantOverride)
	}
}

func TestMaybeCreateSession(t *testing.T) {
	t.Run("skipped when with-session is false", maybeCreateSessionSkipped)
	t.Run("session creation error propagates", maybeCreateSessionError)
	t.Run("success persists session state", maybeCreateSessionSuccess)
	t.Run("state write failure warns but returns nil", maybeCreateSessionWriteFailure)
}

func maybeCreateSessionSkipped(t *testing.T) {
	globals.SetGlobals(t, &config.Config{})
	saveDeployFlags(t)
	withSession = false
	fake := &fakeTarget{name: "gamelift"}
	if err := maybeCreateSession(context.Background(), fake); err != nil {
		t.Fatalf("maybeCreateSession() error = %v", err)
	}
	if fake.sessionCalls != 0 {
		t.Errorf("CreateSession called %d times, want 0", fake.sessionCalls)
	}
}

func maybeCreateSessionError(t *testing.T) {
	globals.SetGlobals(t, &config.Config{})
	saveDeployFlags(t)
	withSession = true
	err := maybeCreateSession(context.Background(), &fakeTarget{sessionErr: fmt.Errorf("create failed")})
	if err == nil {
		t.Fatal("maybeCreateSession() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "session creation failed") {
		t.Errorf("error = %v, want session creation failed", err)
	}
}

func maybeCreateSessionSuccess(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})
	saveDeployFlags(t)
	withSession = true
	fake := &fakeTarget{sessionInfo: &deploy.SessionInfo{SessionID: "sess-9", IPAddress: "10.1.2.3", Port: 7777}}
	if err := maybeCreateSession(context.Background(), fake); err != nil {
		t.Fatalf("maybeCreateSession() error = %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	if s.Session == nil || s.Session.SessionID != "sess-9" || s.Session.IPAddress != "10.1.2.3" {
		t.Errorf("state session = %+v, want sess-9 / 10.1.2.3", s.Session)
	}
}

func maybeCreateSessionWriteFailure(t *testing.T) {
	blockStateWrites(t)
	globals.SetGlobals(t, &config.Config{})
	saveDeployFlags(t)
	withSession = true
	if err := maybeCreateSession(context.Background(), &fakeTarget{}); err != nil {
		t.Fatalf("maybeCreateSession() error = %v", err)
	}
}

func TestMakeDeployer(t *testing.T) {
	t.Run("defaults from config", func(t *testing.T) {
		globals.SetGlobals(t, testGameliftCfg(), globals.WithDryRun(true))
		saveDeployFlags(t)
		d, err := makeDeployer(newCommand())
		if err != nil {
			t.Fatalf("makeDeployer() error = %v", err)
		}
		if d == nil {
			t.Fatal("makeDeployer() returned nil deployer")
		}
	})

	t.Run("flag overrides applied", func(t *testing.T) {
		globals.SetGlobals(t, testGameliftCfg(), globals.WithDryRun(true))
		saveDeployFlags(t)
		region = "eu-west-1"
		instanceType = "c6i.xlarge"
		fleetName = "override-fleet"
		if _, err := makeDeployer(newCommand()); err != nil {
			t.Fatalf("makeDeployer() error = %v", err)
		}
	})

	t.Run("auto-switches instance type for arm64", func(t *testing.T) {
		cfg := testGameliftCfg()
		cfg.Game.Arch = "arm64"
		globals.SetGlobals(t, cfg, globals.WithDryRun(true))
		saveDeployFlags(t)
		instanceType = "c6i.large"
		var runErr error
		out := captureStdout(func() { _, runErr = makeDeployer(newCommand()) })
		if runErr != nil {
			t.Fatalf("makeDeployer() error = %v", runErr)
		}
		if !strings.Contains(out, "Switching instance type") {
			t.Errorf("expected switch note in output, got %q", out)
		}
	})

	t.Run("empty ECR repository errors", func(t *testing.T) {
		cfg := testGameliftCfg()
		cfg.AWS.ECRRepository = ""
		globals.SetGlobals(t, cfg, globals.WithDryRun(true))
		saveDeployFlags(t)
		if _, err := makeDeployer(newCommand()); err == nil {
			t.Fatal("makeDeployer() expected error for empty ECR repository")
		}
	})

	t.Run("aws config resolution error propagates", func(t *testing.T) {
		globals.SetGlobals(t, testGameliftCfg(), globals.WithDryRun(true))
		t.Setenv("AWS_SDK_LOAD_CONFIG", "1")
		t.Setenv("AWS_CONFIG_FILE", t.TempDir())
		saveDeployFlags(t)
		if _, err := makeDeployer(newCommand()); err == nil {
			t.Fatal("makeDeployer() expected aws resolution error")
		}
	})
}

func testGameliftCfg() *config.Config {
	return &config.Config{
		AWS:       config.AWSConfig{Region: "us-east-1", AccountID: "123456789012", ECRRepository: "ludus-server"},
		Container: config.ContainerConfig{Tag: "latest", ServerPort: 7777},
		GameLift:  config.GameLiftConfig{InstanceType: "c6i.large", FleetName: "ludus-fleet", ContainerGroupName: "grp"},
		Game:      config.GameConfig{Arch: "amd64"},
	}
}
