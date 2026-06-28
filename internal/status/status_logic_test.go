package status

import (
	"context"
	"errors"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
)

// stubTarget is a minimal deploy.Target for status tests.
type stubTarget struct {
	status *deploy.DeployStatus
	err    error
}

func (s *stubTarget) Name() string                      { return "stub" }
func (s *stubTarget) Capabilities() deploy.Capabilities { return deploy.Capabilities{} }
func (s *stubTarget) Deploy(context.Context, deploy.DeployInput) (*deploy.DeployResult, error) {
	return nil, nil
}
func (s *stubTarget) Status(context.Context) (*deploy.DeployStatus, error) {
	return s.status, s.err
}
func (s *stubTarget) Destroy(context.Context) error { return nil }

func TestCheckContainerImage_EmptyName(t *testing.T) {
	s := CheckContainerImage("")
	if s.Status != "unknown" {
		t.Errorf("status = %q, want unknown", s.Status)
	}
}

func TestCheckDeployTarget(t *testing.T) {
	ctx := context.Background()

	t.Run("nil target", func(t *testing.T) {
		s := CheckDeployTarget(ctx, nil, "gamelift")
		if s.Status != "unknown" || s.Detail != "target not resolved" {
			t.Errorf("got %+v", s)
		}
	})

	t.Run("empty target name defaults to gamelift", func(t *testing.T) {
		if s := CheckDeployTarget(ctx, nil, ""); s.Name != "Gamelift Deployment" {
			t.Errorf("Name = %q, want 'Gamelift Deployment'", s.Name)
		}
	})

	t.Run("status error", func(t *testing.T) {
		s := CheckDeployTarget(ctx, &stubTarget{err: errors.New("boom")}, "ec2")
		if s.Status != "unknown" {
			t.Errorf("status = %q, want unknown", s.Status)
		}
	})

	t.Run("status mapping", testCheckDeployTargetStatusMapping)
}

func testCheckDeployTargetStatusMapping(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		dsStatus   string
		wantStatus string
	}{
		{"active", "ok"},
		{"not_deployed", "fail"},
		{"weird", "unknown"},
	}
	for _, tt := range tests {
		target := &stubTarget{status: &deploy.DeployStatus{Status: tt.dsStatus}}
		if s := CheckDeployTarget(ctx, target, "stack"); s.Status != tt.wantStatus {
			t.Errorf("ds=%q: status = %q, want %q", tt.dsStatus, s.Status, tt.wantStatus)
		}
	}
}

func TestCheckAll_ReturnsAllStages(t *testing.T) {
	t.Chdir(t.TempDir()) // isolate state.Load
	cfg := &config.Config{}
	cfg.Game.ProjectName = "Lyra"
	target := &stubTarget{status: &deploy.DeployStatus{Status: "not_deployed"}}

	stages := CheckAll(context.Background(), cfg, target)
	if len(stages) != 7 {
		t.Fatalf("CheckAll returned %d stages, want 7", len(stages))
	}
	for _, st := range stages {
		if st.Name == "" {
			t.Errorf("stage with empty name: %+v", st)
		}
	}
}
