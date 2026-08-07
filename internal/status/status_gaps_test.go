package status

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// writeCorruptState drops an unparsable state.json into the cwd's .ludus folder
// so state.Load() returns an error.
func writeCorruptState(t *testing.T) {
	t.Helper()
	dir := chdirTemp(t)
	if err := os.MkdirAll(filepath.Join(dir, ".ludus"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ludus", "state.json"), []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckClientBuild_StateReadError(t *testing.T) {
	writeCorruptState(t)

	s := CheckClientBuild("TestGame")
	if s.Status != "unknown" || s.Detail != "could not read state" {
		t.Errorf("got %+v, want unknown/could not read state", s)
	}
}

func TestCheckGameSession_StateReadError(t *testing.T) {
	writeCorruptState(t)

	cfg := &config.Config{Deploy: config.DeployConfig{Target: "gamelift"}}
	s := CheckGameSession(cfg)
	if s.Status != "unknown" || s.Detail != "could not read state" {
		t.Errorf("got %+v, want unknown/could not read state", s)
	}
}

func TestCheckGameSession_DetailFormatting(t *testing.T) {
	dir := chdirTemp(t)
	writeState(t, dir, &state.State{
		Session: &state.SessionState{
			SessionID: "gsess-123",
			IPAddress: "1.2.3.4",
			Port:      7777,
		},
	})

	cfg := &config.Config{Deploy: config.DeployConfig{Target: "gamelift"}}
	s := CheckGameSession(cfg)
	if s.Detail != "gsess-123 (1.2.3.4:7777)" {
		t.Errorf("detail = %q, want 'gsess-123 (1.2.3.4:7777)'", s.Detail)
	}
}

func TestCheckAll_ProjectPathOutputDir(t *testing.T) {
	dir := chdirTemp(t)
	writeState(t, dir, &state.State{})
	cfg := &config.Config{}
	cfg.Game.ProjectName = "MyGame"
	cfg.Game.ProjectPath = filepath.Join(dir, "MyGame.uproject")

	stages := CheckAll(context.Background(), cfg, &stubTarget{status: &deploy.DeployStatus{Status: "not_deployed"}})
	found := false
	for _, st := range stages {
		if st.Name == "MyGame Server Build" {
			found = true
			if st.Status != "fail" || st.Detail != "not built" {
				t.Errorf("got %+v, want fail/not built", st)
			}
		}
	}
	if !found {
		t.Error("expected a 'MyGame Server Build' stage")
	}
}

func TestCheckAll_LyraFallbackOutputDir(t *testing.T) {
	dir := chdirTemp(t)
	writeState(t, dir, &state.State{})
	cfg := &config.Config{}
	cfg.Engine.SourcePath = t.TempDir()
	cfg.Game.ProjectName = "Lyra"

	stages := CheckAll(context.Background(), cfg, &stubTarget{status: &deploy.DeployStatus{Status: "not_deployed"}})
	found := false
	for _, st := range stages {
		if st.Name == "Lyra Server Build" {
			found = true
			if st.Status != "fail" || st.Detail != "not built" {
				t.Errorf("got %+v, want fail/not built", st)
			}
		}
	}
	if !found {
		t.Error("expected a 'Lyra Server Build' stage")
	}
}

func TestCheckContainerImage_ExecPaths(t *testing.T) {
	tests := []struct {
		name     string
		behavior testsupport.ToolBehavior
		want     string
		wantMsg  string
	}{
		{
			name:     "docker unavailable",
			behavior: testsupport.ToolBehavior{ExitCode: 1},
			want:     "unknown",
			wantMsg:  "docker not available",
		},
		{
			name:     "no image found",
			behavior: testsupport.ToolBehavior{Stdout: ""},
			want:     "fail",
			wantMsg:  "no image found",
		},
		{
			name:     "image exists",
			behavior: testsupport.ToolBehavior{Stdout: "latest"},
			want:     "ok",
			wantMsg:  "tags: latest",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTool(t, "docker", tt.behavior)
			s := CheckContainerImage("ludus-server")
			if s.Status != tt.want {
				t.Errorf("status = %q, want %q", s.Status, tt.want)
			}
			if !strings.Contains(s.Detail, tt.wantMsg) {
				t.Errorf("detail %q missing %q", s.Detail, tt.wantMsg)
			}
		})
	}
}
