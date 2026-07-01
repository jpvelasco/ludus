package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestHandleStatusReturnsPipelineAndSecurityResults(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	previous := globals.Cfg
	cfg := config.Defaults()
	cfg.Deploy.Target = "binary"
	cfg.Engine.SourcePath = t.TempDir()
	cfg.Game.ProjectPath = t.TempDir() + "/Lyra.uproject"
	globals.Cfg = cfg
	t.Cleanup(func() { globals.Cfg = previous })

	result, _, err := handleStatus(context.Background(), nil, statusInput{})
	if err != nil {
		t.Fatalf("handleStatus: %v", err)
	}
	var got statusResult
	if err := json.Unmarshal([]byte(toolResultText(t, result)), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(got.Stages) != 7 {
		t.Errorf("stage count = %d, want 7", len(got.Stages))
	}
	if len(got.Security) != 3 {
		t.Errorf("security scan count = %d, want 3", len(got.Security))
	}
}

func TestHandleStatusContinuesWhenTargetCannotResolve(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	previous := globals.Cfg
	cfg := config.Defaults()
	cfg.Deploy.Target = "unknown"
	globals.Cfg = cfg
	t.Cleanup(func() { globals.Cfg = previous })

	result, _, err := handleStatus(context.Background(), nil, statusInput{})
	if err != nil {
		t.Fatalf("handleStatus: %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "target not resolved") {
		t.Fatalf("result = %q, want unresolved target stage", text)
	}
}

func TestRunSecurityScans(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cfg := config.Defaults()
	cfg.Game.ProjectName = "Lyra"
	cfg.Game.ServerTarget = "LyraServer"
	cfg.Container.ImageName = "example/server"
	cfg.Container.Tag = "test"

	scans := runSecurityScans(cfg)
	if len(scans) != 3 {
		t.Fatalf("scan count = %d, want 3", len(scans))
	}
	wants := []string{"Game Dockerfile", "Engine Dockerfile", "Container Image (example/server:test)"}
	for i, want := range wants {
		if scans[i].Target != want {
			t.Errorf("scan %d target = %q, want %q", i, scans[i].Target, want)
		}
		if strings.TrimSpace(scans[i].Summary) == "" {
			t.Errorf("scan %d has an empty summary", i)
		}
	}
}
