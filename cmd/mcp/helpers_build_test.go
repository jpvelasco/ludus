package mcp

import (
	"testing"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
)

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
			info := estimateCost(tt.instanceType, tt.arch)
			_ = info.EstimatedCostPerHour
			_ = info.InstanceGuidance
		})
	}
}
