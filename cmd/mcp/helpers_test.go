package mcp

import (
	"testing"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestOverridesDoNotMutateGlobal(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })

	globals.Cfg = &config.Config{
		AWS:      config.AWSConfig{Region: "us-east-1"},
		GameLift: config.GameLiftConfig{InstanceType: "c6i.large", FleetName: "original-fleet"},
		Game:     config.GameConfig{Arch: "amd64"},
		Anywhere: config.AnywhereConfig{IPAddress: "10.0.0.1"},
	}

	// Simulate the handler pattern: value copy + overrides
	cfg := globals.Cfg.Clone()
	applyRegionOverride(&cfg, "eu-west-1")
	applyInstanceOverride(&cfg, "c7g.large")
	applyFleetNameOverride(&cfg, "new-fleet")
	applyArchOverride(&cfg, "arm64")
	cfg.Anywhere.IPAddress = "192.168.1.1"

	assertIsolated(t, "Region", cfg.AWS.Region, "eu-west-1", globals.Cfg.AWS.Region, "us-east-1")
	assertIsolated(t, "InstanceType", cfg.GameLift.InstanceType, "c7g.large", globals.Cfg.GameLift.InstanceType, "c6i.large")
	assertIsolated(t, "FleetName", cfg.GameLift.FleetName, "new-fleet", globals.Cfg.GameLift.FleetName, "original-fleet")
	assertIsolated(t, "Arch", cfg.Game.Arch, "arm64", globals.Cfg.Game.Arch, "amd64")
	assertIsolated(t, "IPAddress", cfg.Anywhere.IPAddress, "192.168.1.1", globals.Cfg.Anywhere.IPAddress, "10.0.0.1")
}

func TestDockerDispatchUsesIsolatedConfig(t *testing.T) {
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })

	globals.Cfg = &config.Config{
		Game:   config.GameConfig{Arch: "amd64", ProjectName: "Lyra"},
		Engine: config.EngineConfig{SourcePath: "/engine", Version: "5.7"},
	}

	// Simulate handleGameBuild: value copy, arch override, then dispatch
	cfg := globals.Cfg.Clone()
	applyArchOverride(&cfg, "arm64")

	// The cfg passed to handleDockerGameBuild should have arm64
	assertIsolated(t, "Arch", cfg.Game.Arch, "arm64", globals.Cfg.Game.Arch, "amd64")

	// Cache keys must differ when arch differs (proves sub-handler would
	// compute different keys from the isolated config vs the global)
	localKey := cache.GameServerKey(&cfg, cache.EngineKey(&cfg))
	globalKey := cache.GameServerKey(globals.Cfg, cache.EngineKey(globals.Cfg))
	if localKey == globalKey {
		t.Error("cache keys should differ between isolated cfg (arm64) and global (amd64)")
	}
}

func TestMergeOutput(t *testing.T) {
	tests := []struct {
		name   string
		stdout string
		stderr string
		want   string
	}{
		{"both present", "out", "err", "outerr"},
		{"stdout only", "output", "", "output"},
		{"stderr only", "", "error", "error"},
		{"both empty", "", "", ""},
		{"multiline", "line1\nline2\n", "warn\n", "line1\nline2\nwarn\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := capturedOutput{Stdout: tt.stdout, Stderr: tt.stderr}
			got := mergeOutput(c)
			if got != tt.want {
				t.Errorf("mergeOutput(%q, %q) = %q, want %q", tt.stdout, tt.stderr, got, tt.want)
			}
		})
	}
}

func TestResultOK(t *testing.T) {
	type payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	v := payload{Success: true, Message: "done"}

	result, structured, err := resultOK(v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Errorf("expected nil structured result, got %v", structured)
	}
	if result.IsError {
		t.Error("expected IsError = false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected *mcpsdk.TextContent, got %T", result.Content[0])
	}
	if tc.Text == "" {
		t.Error("expected non-empty text content")
	}
}

func TestResultErr(t *testing.T) {
	type payload struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	v := payload{Success: false, Error: "something failed"}

	result, structured, err := resultErr(v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Errorf("expected nil structured result, got %v", structured)
	}
	if !result.IsError {
		t.Error("expected IsError = true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
}

func TestNewToolRunner(t *testing.T) {
	origDryRun := globals.DryRun
	defer func() { globals.DryRun = origDryRun }()

	tests := []struct {
		name       string
		inputDry   bool
		globalDry  bool
		wantDryRun bool
	}{
		{"input dry run", true, false, true},
		{"global dry run", false, true, true},
		{"both dry run", true, true, true},
		{"neither dry run", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals.DryRun = tt.globalDry
			r := newToolRunner(tt.inputDry)
			if r == nil {
				t.Fatal("expected non-nil runner")
			}
			if !r.Verbose {
				t.Error("expected Verbose = true for MCP runner")
			}
			if r.DryRun != tt.wantDryRun {
				t.Errorf("DryRun = %v, want %v", r.DryRun, tt.wantDryRun)
			}
		})
	}
}

type wantBuildResult struct {
	status    string
	buildType string
	buildID   string
	hasOutput bool
	errMsg    string
}

func assertBuildResult(t *testing.T, r buildStatusResult, want wantBuildResult) {
	t.Helper()
	if r.BuildID != want.buildID {
		t.Errorf("BuildID = %q, want %q", r.BuildID, want.buildID)
	}
	if r.Status != want.status {
		t.Errorf("Status = %q, want %q", r.Status, want.status)
	}
	if r.Type != want.buildType {
		t.Errorf("Type = %q, want %q", r.Type, want.buildType)
	}
	if r.ElapsedSeconds <= 0 {
		t.Errorf("ElapsedSeconds = %f, want > 0", r.ElapsedSeconds)
	}
	if r.Error != want.errMsg {
		t.Errorf("Error = %q, want %q", r.Error, want.errMsg)
	}
	hasOutput := r.OutputTail != ""
	if hasOutput != want.hasOutput {
		t.Errorf("has OutputTail = %v, want %v", hasOutput, want.hasOutput)
	}
}

func TestBuildEntryToResult(t *testing.T) {
	now := time.Now()
	buf := &syncBuffer{}
	_, _ = buf.Write([]byte("line1\nline2\nline3\n"))

	t.Run("running build summary", func(t *testing.T) {
		entry := &buildEntry{
			ID: "engine_build-20260331-100000", Type: buildTypeEngineBuild,
			Status: buildStatusRunning, StartedAt: now.Add(-5 * time.Second), Output: buf,
		}
		assertBuildResult(t, buildEntryToResult(entry, false), wantBuildResult{
			status: "running", buildType: "engine_build", buildID: entry.ID,
		})
	})

	t.Run("completed build detailed", func(t *testing.T) {
		entry := &buildEntry{
			ID: "game_build-20260331-100000", Type: buildTypeGameBuild,
			Status: buildStatusCompleted, StartedAt: now.Add(-10 * time.Second), EndedAt: now,
			Result: map[string]string{"path": "/builds/server"}, Output: buf,
		}
		assertBuildResult(t, buildEntryToResult(entry, true), wantBuildResult{
			status: "completed", buildType: "game_build", buildID: entry.ID, hasOutput: true,
		})
	})

	t.Run("failed build with error", func(t *testing.T) {
		entry := &buildEntry{
			ID: "game_client-20260331-100000", Type: buildTypeGameClient,
			Status: buildStatusFailed, StartedAt: now.Add(-3 * time.Second), EndedAt: now,
			Error: "compilation failed", Output: buf,
		}
		assertBuildResult(t, buildEntryToResult(entry, true), wantBuildResult{
			status: "failed", buildType: "game_client", buildID: entry.ID,
			hasOutput: true, errMsg: "compilation failed",
		})
	})

	t.Run("cancelled build summary", func(t *testing.T) {
		entry := &buildEntry{
			ID: "engine_build-20260331-110000", Type: buildTypeEngineBuild,
			Status: buildStatusCancelled, StartedAt: now.Add(-1 * time.Second), EndedAt: now,
			Output: &syncBuffer{},
		}
		assertBuildResult(t, buildEntryToResult(entry, false), wantBuildResult{
			status: "cancelled", buildType: "engine_build", buildID: entry.ID,
		})
	})
}

func TestEstimateCost(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		arch         string
	}{
		{"known instance", "c6i.large", "amd64"},
		{"graviton instance", "c7g.large", "arm64"},
		{"unknown instance", "z99.mega", "amd64"},
		{"empty strings", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify it returns without panicking
			info := estimateCost(tt.instanceType, tt.arch)
			_ = info.EstimatedCostPerHour
			_ = info.InstanceGuidance
		})
	}
}
