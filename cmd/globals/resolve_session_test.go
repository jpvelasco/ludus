package globals

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

// writeRawState writes raw bytes to the default profile's state file so tests
// can install malformed content that state.Load rejects.
func writeRawState(t *testing.T, data string) {
	t.Helper()
	if err := os.MkdirAll(".ludus", 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(".ludus", "state.json"), []byte(data), 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}
}

// blockStateReads makes state.Load fail by placing a directory where the
// state.json file should be: reading it errors on every platform without ever
// classifying as os.IsNotExist (a regular-file .ludus blocker reads as
// ERROR_PATH_NOT_FOUND on Windows, which state.Load treats as "no state yet").
func blockStateReads(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(".ludus", "state.json"), 0o755); err != nil {
		t.Fatalf("create state.json blocker: %v", err)
	}
}

// TestResolveSessionTarget_StateLoadErrors pins #694: a state file that cannot
// be read or parsed must surface as a contextual error instead of a nil-deref
// panic on the state-inspection path.
func TestResolveSessionTarget_StateLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		session bool
	}{
		{
			name:  "corrupt JSON with session-incapable config target",
			setup: func(t *testing.T) { writeRawState(t, "{") },
		},
		{
			name:  "unreadable state path with session-incapable config target",
			setup: blockStateReads,
		},
		{
			name:    "corrupt JSON even when the config target supports sessions",
			setup:   func(t *testing.T) { writeRawState(t, "{") },
			session: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			tt.setup(t)
			if tt.session {
				SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
					return &sessionStubTarget{swapStubTarget{name: "session"}}, nil
				})
			}
			SetGlobals(t, &config.Config{Deploy: config.DeployConfig{Target: "binary"}})

			_, err := ResolveSessionTarget(context.Background(), Cfg)
			if err == nil {
				t.Fatal("ResolveSessionTarget() wanted a state-loading error, got nil")
			}
			if !strings.Contains(err.Error(), "deploy state") {
				t.Errorf("ResolveSessionTarget() error = %v, want a contextual state-loading error", err)
			}
		})
	}
}

// TestResolveSessionTarget_PrefersRecordedDeployment pins #735: when state
// records a deployment, session resolution must target it even if the
// configured target also supports sessions.
func TestResolveSessionTarget_PrefersRecordedDeployment(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "ec2",
		Status:     "ACTIVE",
		DeployedAt: "2026-10-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpdateDeploy: %v", err)
	}

	var gotOverride string
	SwapResolveTarget(t, func(_ context.Context, _ *config.Config, override string) (deploy.Target, error) {
		gotOverride = override
		return &sessionStubTarget{swapStubTarget{name: "session"}}, nil
	})

	cfg := &config.Config{Deploy: config.DeployConfig{Target: "gamelift"}}
	target, err := ResolveSessionTarget(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ResolveSessionTarget: %v", err)
	}
	if gotOverride != "ec2" {
		t.Errorf("ResolveTarget override = %q, want the recorded target %q", gotOverride, "ec2")
	}
	if target.Name() != "session" {
		t.Errorf("target.Name() = %q, want the target resolved from state", target.Name())
	}
}

// TestResolveSessionTarget_MissingRecordUsesConfig pins the missing-state half
// of the resolution order: with no recorded deployment the configured target
// is resolved.
func TestResolveSessionTarget_MissingRecordUsesConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	var gotOverride string
	SwapResolveTarget(t, func(_ context.Context, _ *config.Config, override string) (deploy.Target, error) {
		gotOverride = override
		return &sessionStubTarget{swapStubTarget{name: "session"}}, nil
	})

	cfg := &config.Config{Deploy: config.DeployConfig{Target: "gamelift"}}
	if _, err := ResolveSessionTarget(context.Background(), cfg); err != nil {
		t.Fatalf("ResolveSessionTarget: %v", err)
	}
	if gotOverride != "" {
		t.Errorf("ResolveTarget override = %q, want empty (config default)", gotOverride)
	}
}

// TestResolveSessionTarget_StaleRecordErrors pins the stale-state half of the
// resolution order: a recorded target that no longer resolves fails loudly
// instead of silently falling back to the configured target.
func TestResolveSessionTarget_StaleRecordErrors(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "nonexistent-target",
		Status:     "ACTIVE",
		DeployedAt: "2026-10-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpdateDeploy: %v", err)
	}

	_, err := ResolveSessionTarget(context.Background(), &config.Config{
		Deploy: config.DeployConfig{Target: "binary"},
	})
	if err == nil {
		t.Fatal("ResolveSessionTarget() wanted an error for a stale recorded target, got nil")
	}
	if !strings.Contains(err.Error(), "from state") {
		t.Errorf("ResolveSessionTarget() error = %v, want a state-resolution error", err)
	}
}
