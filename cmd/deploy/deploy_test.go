package deploy

import (
	"testing"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
)

func TestApplyFlagsDoNotMutateGlobal(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })

	globals.Cfg = &config.Config{
		AWS:      config.AWSConfig{Region: "us-east-1"},
		GameLift: config.GameLiftConfig{InstanceType: "c6i.large", FleetName: "original"},
		Game:     config.GameConfig{Arch: "amd64"},
		Anywhere: config.AnywhereConfig{IPAddress: "10.0.0.1"},
	}

	t.Run("applyEC2Flags isolates mutations", func(t *testing.T) {
		origRegion, origInstance, origFleet, origArch := region, instanceType, fleetName, ec2Arch
		t.Cleanup(func() { region, instanceType, fleetName, ec2Arch = origRegion, origInstance, origFleet, origArch })

		region = "eu-west-1"
		instanceType = "c7g.large"
		fleetName = "new-fleet"
		ec2Arch = "arm64"

		cfg := *globals.Cfg
		applyEC2Flags(&cfg)

		if cfg.AWS.Region != "eu-west-1" {
			t.Errorf("local Region = %q, want %q", cfg.AWS.Region, "eu-west-1")
		}
		if cfg.Game.Arch != "arm64" {
			t.Errorf("local Arch = %q, want %q", cfg.Game.Arch, "arm64")
		}
		if globals.Cfg.AWS.Region != "us-east-1" {
			t.Errorf("global Region mutated: got %q, want %q", globals.Cfg.AWS.Region, "us-east-1")
		}
		if globals.Cfg.Game.Arch != "amd64" {
			t.Errorf("global Arch mutated: got %q, want %q", globals.Cfg.Game.Arch, "amd64")
		}
	})

	t.Run("applyAnywhereFlags isolates mutations", func(t *testing.T) {
		origRegion, origFleet, origIP := region, fleetName, anywhereIP
		t.Cleanup(func() { region, fleetName, anywhereIP = origRegion, origFleet, origIP })

		region = "ap-southeast-1"
		fleetName = "anywhere-fleet"
		anywhereIP = "192.168.1.1"

		cfg := *globals.Cfg
		applyAnywhereFlags(&cfg)

		if cfg.Anywhere.IPAddress != "192.168.1.1" {
			t.Errorf("local IPAddress = %q, want %q", cfg.Anywhere.IPAddress, "192.168.1.1")
		}
		if globals.Cfg.Anywhere.IPAddress != "10.0.0.1" {
			t.Errorf("global IPAddress mutated: got %q, want %q", globals.Cfg.Anywhere.IPAddress, "10.0.0.1")
		}
	})

	t.Run("applyStackFlags isolates mutations", func(t *testing.T) {
		origRegion, origInstance := region, instanceType
		t.Cleanup(func() { region, instanceType = origRegion, origInstance })

		region = "us-west-2"
		instanceType = "m5.xlarge"

		cfg := *globals.Cfg
		applyStackFlags(&cfg)

		if cfg.AWS.Region != "us-west-2" {
			t.Errorf("local Region = %q, want %q", cfg.AWS.Region, "us-west-2")
		}
		if globals.Cfg.AWS.Region != "us-east-1" {
			t.Errorf("global Region mutated: got %q, want %q", globals.Cfg.AWS.Region, "us-east-1")
		}
		if globals.Cfg.GameLift.InstanceType != "c6i.large" {
			t.Errorf("global InstanceType mutated: got %q, want %q", globals.Cfg.GameLift.InstanceType, "c6i.large")
		}
	})
}
