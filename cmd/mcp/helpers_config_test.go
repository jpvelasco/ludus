package mcp

import (
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestResolveBackend(t *testing.T) {
	tests := []struct {
		name          string
		inputBackend  string
		configBackend string
		want          string
	}{
		{"input takes precedence", "docker", "native", "docker"},
		{"falls back to config", "", "native", "native"},
		{"both empty", "", "", ""},
		{"input only", "docker", "", "docker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBackend(tt.inputBackend, tt.configBackend)
			if got != tt.want {
				t.Errorf("resolveBackend(%q, %q) = %q, want %q", tt.inputBackend, tt.configBackend, got, tt.want)
			}
		})
	}
}

func TestApplyRegionOverride(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		override   string
		wantRegion string
	}{
		{"applies override", "us-east-1", "eu-west-1", "eu-west-1"},
		{"no-op when empty", "us-east-1", "", "us-east-1"},
		{"sets when initially empty", "", "ap-southeast-1", "ap-southeast-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{AWS: config.AWSConfig{Region: tt.initial}}
			applyRegionOverride(cfg, tt.override)
			if cfg.AWS.Region != tt.wantRegion {
				t.Errorf("Region = %q, want %q", cfg.AWS.Region, tt.wantRegion)
			}
		})
	}
}

func TestApplyInstanceOverride(t *testing.T) {
	tests := []struct {
		name         string
		initial      string
		override     string
		wantInstance string
	}{
		{"applies override", "c6i.large", "c7g.large", "c7g.large"},
		{"no-op when empty", "c6i.large", "", "c6i.large"},
		{"sets when initially empty", "", "m5.xlarge", "m5.xlarge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{GameLift: config.GameLiftConfig{InstanceType: tt.initial}}
			applyInstanceOverride(cfg, tt.override)
			if cfg.GameLift.InstanceType != tt.wantInstance {
				t.Errorf("InstanceType = %q, want %q", cfg.GameLift.InstanceType, tt.wantInstance)
			}
		})
	}
}

func TestApplyFleetNameOverride(t *testing.T) {
	tests := []struct {
		name      string
		initial   string
		override  string
		wantFleet string
	}{
		{"applies override", "ludus-fleet", "my-fleet", "my-fleet"},
		{"no-op when empty", "ludus-fleet", "", "ludus-fleet"},
		{"sets when initially empty", "", "custom-fleet", "custom-fleet"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{GameLift: config.GameLiftConfig{FleetName: tt.initial}}
			applyFleetNameOverride(cfg, tt.override)
			if cfg.GameLift.FleetName != tt.wantFleet {
				t.Errorf("FleetName = %q, want %q", cfg.GameLift.FleetName, tt.wantFleet)
			}
		})
	}
}

func TestApplyArchOverride(t *testing.T) {
	tests := []struct {
		name     string
		initial  string
		override string
		wantArch string
	}{
		{"applies override", "amd64", "arm64", "arm64"},
		{"no-op when empty", "amd64", "", "amd64"},
		{"sets when initially empty", "", "arm64", "arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Game: config.GameConfig{Arch: tt.initial}}
			applyArchOverride(cfg, tt.override)
			if cfg.Game.Arch != tt.wantArch {
				t.Errorf("Arch = %q, want %q", cfg.Game.Arch, tt.wantArch)
			}
		})
	}
}

func assertIsolated(t *testing.T, field, local, wantLocal, global, wantGlobal string) {
	t.Helper()
	if local != wantLocal {
		t.Errorf("local %s = %q, want %q", field, local, wantLocal)
	}
	if global != wantGlobal {
		t.Errorf("global %s mutated: got %q, want %q", field, global, wantGlobal)
	}
}

func TestIsolatedConfig(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })

	globals.Cfg = &config.Config{
		AWS:      config.AWSConfig{Region: "us-east-1"},
		GameLift: config.GameLiftConfig{InstanceType: "c6i.large", FleetName: "original-fleet"},
		Game:     config.GameConfig{Arch: "amd64"},
		Anywhere: config.AnywhereConfig{IPAddress: "10.0.0.1"},
	}

	cfg := isolatedConfig(deployOverrides{
		Region:       "eu-west-1",
		InstanceType: "c7g.large",
		FleetName:    "new-fleet",
		Arch:         "arm64",
		IPAddress:    "192.168.1.1",
	})

	assertIsolated(t, "Region", cfg.AWS.Region, "eu-west-1", globals.Cfg.AWS.Region, "us-east-1")
	assertIsolated(t, "InstanceType", cfg.GameLift.InstanceType, "c7g.large", globals.Cfg.GameLift.InstanceType, "c6i.large")
	assertIsolated(t, "FleetName", cfg.GameLift.FleetName, "new-fleet", globals.Cfg.GameLift.FleetName, "original-fleet")
	assertIsolated(t, "Arch", cfg.Game.Arch, "arm64", globals.Cfg.Game.Arch, "amd64")
	assertIsolated(t, "IPAddress", cfg.Anywhere.IPAddress, "192.168.1.1", globals.Cfg.Anywhere.IPAddress, "10.0.0.1")
}

func TestIsolatedConfigEmptyOverrides(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })

	globals.Cfg = &config.Config{
		AWS:      config.AWSConfig{Region: "us-east-1"},
		GameLift: config.GameLiftConfig{InstanceType: "c6i.large"},
	}

	cfg := isolatedConfig(deployOverrides{})

	if cfg.AWS.Region != "us-east-1" {
		t.Errorf("Region = %q, want %q", cfg.AWS.Region, "us-east-1")
	}
	if cfg.GameLift.InstanceType != "c6i.large" {
		t.Errorf("InstanceType = %q, want %q", cfg.GameLift.InstanceType, "c6i.large")
	}
}

func TestDockerDispatchUsesIsolatedConfig(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })

	globals.Cfg = &config.Config{
		Game:   config.GameConfig{Arch: "amd64", ProjectName: "Lyra"},
		Engine: config.EngineConfig{SourcePath: "/engine", Version: "5.7"},
	}

	cfg := globals.Cfg.Clone()
	applyArchOverride(&cfg, "arm64")

	assertIsolated(t, "Arch", cfg.Game.Arch, "arm64", globals.Cfg.Game.Arch, "amd64")

	localKey := cache.GameServerKey(&cfg, cache.EngineKey(&cfg))
	globalKey := cache.GameServerKey(globals.Cfg, cache.EngineKey(globals.Cfg))
	if localKey == globalKey {
		t.Error("cache keys should differ between isolated cfg (arm64) and global (amd64)")
	}
}
