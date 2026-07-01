package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func decodeToolJSON(t *testing.T, result *mcpsdk.CallToolResult, dst any) {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("expected tool content")
	}
	text, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want TextContent", result.Content[0])
	}
	if err := json.Unmarshal([]byte(text.Text), dst); err != nil {
		t.Fatalf("decode tool JSON: %v", err)
	}
}

func TestSnapshotConfigDeepCopiesReferences(t *testing.T) {
	orig := globals.Cfg
	t.Cleanup(func() { globals.Cfg = orig })
	globals.Cfg = &config.Config{
		AWS: config.AWSConfig{Tags: map[string]string{"env": "test"}},
		Game: config.GameConfig{ContentValidation: &config.ContentValidationConfig{
			ContentMarkerFile: "marker", PluginContentDirs: []string{"plugin"},
		}},
		CI: config.CIConfig{RunnerLabels: []string{"linux"}},
	}

	snap := snapshotConfig()
	snap.AWS.Tags["env"] = "changed"
	snap.Game.ContentValidation.ContentMarkerFile = "changed"
	snap.Game.ContentValidation.PluginContentDirs[0] = "changed"
	snap.CI.RunnerLabels[0] = "changed"

	checks := map[string]string{
		"tag":    globals.Cfg.AWS.Tags["env"],
		"marker": globals.Cfg.Game.ContentValidation.ContentMarkerFile,
		"plugin": globals.Cfg.Game.ContentValidation.PluginContentDirs[0],
		"label":  globals.Cfg.CI.RunnerLabels[0],
	}
	for name, got := range checks {
		if got == "changed" {
			t.Errorf("%s shared storage with snapshot", name)
		}
	}
}

func TestHandleBuildStatusManagerUnavailable(t *testing.T) {
	previous := builds
	builds = nil
	t.Cleanup(func() { builds = previous })
	result, _, err := handleBuildStatus(context.Background(), nil, buildStatusInput{})
	assertToolError(t, result, err, "not initialized")
}

func TestHandleBuildStatusModes(t *testing.T) {
	withBuildManager(t)
	started := make(chan struct{})
	id, err := builds.Start(buildTypeEngineBuild, func(ctx context.Context, buf *syncBuffer) (any, error) {
		_, _ = buf.Write([]byte(strings.Repeat("line\n", 105)))
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	<-started
	t.Run("unknown", func(t *testing.T) { testBuildStatusUnknown(t) })
	t.Run("list", func(t *testing.T) { testBuildStatusList(t, id) })
	t.Run("detail truncates output", func(t *testing.T) { testBuildStatusDetail(t, id) })
	t.Run("cancel", func(t *testing.T) { testBuildStatusCancel(t, id) })
	t.Run("cancel stale", func(t *testing.T) { testBuildStatusStale(t, id) })
}

func testBuildStatusUnknown(t *testing.T) {
	result, _, err := handleBuildStatus(context.Background(), nil, buildStatusInput{BuildID: "missing"})
	assertToolError(t, result, err, "not found")
}

func testBuildStatusList(t *testing.T, id string) {
	result, _, err := handleBuildStatus(context.Background(), nil, buildStatusInput{})
	if err != nil {
		t.Fatalf("handleBuildStatus error: %v", err)
	}
	var got buildListResult
	decodeToolJSON(t, result, &got)
	if len(got.Builds) != 1 || got.Builds[0].BuildID != id {
		t.Fatalf("build list = %+v", got.Builds)
	}
	if got.Builds[0].OutputTail != "" {
		t.Error("list response should omit output tail")
	}
}

func testBuildStatusDetail(t *testing.T, id string) {
	result, _, err := handleBuildStatus(context.Background(), nil, buildStatusInput{BuildID: id})
	if err != nil {
		t.Fatalf("handleBuildStatus error: %v", err)
	}
	var got buildStatusResult
	decodeToolJSON(t, result, &got)
	if strings.Count(got.OutputTail, "line") != 100 {
		t.Errorf("output lines = %d, want 100", strings.Count(got.OutputTail, "line"))
	}
	if got.OutputBytes != 525 {
		t.Errorf("OutputBytes = %d, want 525", got.OutputBytes)
	}
}

func testBuildStatusCancel(t *testing.T, id string) {
	result, _, err := handleBuildStatus(context.Background(), nil, buildStatusInput{BuildID: id, Cancel: true})
	if err != nil || result.IsError {
		t.Fatalf("cancel result error: %v, %+v", err, result)
	}
	waitForStatus(t, builds, id, buildStatusCancelled)
}

func testBuildStatusStale(t *testing.T, id string) {
	result, _, err := handleBuildStatus(context.Background(), nil, buildStatusInput{BuildID: id, Cancel: true})
	assertToolError(t, result, err, "not running")
}

func TestBuildEntryToResultCompletedDuration(t *testing.T) {
	start := time.Now().Add(-2 * time.Second)
	entry := &buildEntry{
		ID: "done", Type: buildTypeGameBuild, Status: buildStatusCompleted,
		StartedAt: start, EndedAt: start.Add(1500 * time.Millisecond), Output: &syncBuffer{},
	}
	got := buildEntryToResult(entry, true)
	if got.ElapsedSeconds != 1.5 {
		t.Errorf("ElapsedSeconds = %v, want 1.5", got.ElapsedSeconds)
	}
}
