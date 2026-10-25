package globals

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/stack"
	"github.com/jpvelasco/ludus/internal/state"
)

// stackCfg builds a config resolveStack can resolve offline in dry-run mode:
// region and account come from config, and the deployer constructor is pure.
func stackCfg() *config.Config {
	return &config.Config{
		AWS:       config.AWSConfig{Region: "us-west-2", AccountID: "123456789012", ECRRepository: "ludus-server"},
		Container: config.ContainerConfig{Tag: "latest", ServerPort: 7777},
		GameLift:  config.GameLiftConfig{FleetName: "ludus-fleet", InstanceType: "c6i.large", ContainerGroupName: "ludus-container-group"},
		Game:      config.GameConfig{Arch: "amd64"},
		EC2Fleet:  config.EC2FleetConfig{},
	}
}

// recordDeployment writes fleet and deploy state as the target adapters do
// after a successful deployment.
func recordDeployment(t *testing.T, targetName, stackName string) {
	t.Helper()
	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   "fleet-1",
		StackName: stackName,
		Status:    "ACTIVE",
		CreatedAt: "2026-10-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpdateFleet: %v", err)
	}
	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: targetName,
		Status:     "ACTIVE",
		DeployedAt: "2026-10-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpdateDeploy: %v", err)
	}
}

// resolveStackTarget resolves the stack target offline and returns its
// resolved options.
func resolveStackTarget(t *testing.T, cfg *config.Config) stack.StackOptions {
	t.Helper()
	SetGlobals(t, cfg, WithDryRun(true))
	target, err := ResolveTarget(context.Background(), cfg, "stack")
	if err != nil {
		t.Fatalf("ResolveTarget(stack): %v", err)
	}
	adapter, ok := target.(*stack.TargetAdapter)
	if !ok {
		t.Fatalf("ResolveTarget(stack) = %T, want *stack.TargetAdapter", target)
	}
	return adapter.Deployer().Options()
}

// TestRecordedStackName pins the state-reading half of #737: the recorded
// stack name only counts for the most recent stack deployment, and unreadable
// state errors instead of silently switching stacks.
func TestRecordedStackName(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		want    string
		wantErr bool
	}{
		{
			name:  "no state",
			setup: func(*testing.T) {},
			want:  "",
		},
		{
			name:  "recorded stack deployment",
			setup: func(t *testing.T) { recordDeployment(t, "stack", "custom-stack") },
			want:  "custom-stack",
		},
		{
			name:  "latest deployment is not a stack",
			setup: func(t *testing.T) { recordDeployment(t, "gamelift", "stale-stack") },
			want:  "",
		},
		{
			name: "fleet record missing",
			setup: func(t *testing.T) {
				if err := state.UpdateDeploy(&state.DeployState{TargetName: "stack", Status: "ACTIVE"}); err != nil {
					t.Fatalf("UpdateDeploy: %v", err)
				}
			},
			want: "",
		},
		{
			name:    "corrupt state errors",
			setup:   func(t *testing.T) { writeRawState(t, "{") },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			tt.setup(t)

			got, err := recordedStackName()
			if (err != nil) != tt.wantErr {
				t.Fatalf("recordedStackName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "deploy state") {
				t.Errorf("recordedStackName() error = %v, want a contextual state error", err)
			}
			if got != tt.want {
				t.Errorf("recordedStackName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveTarget_StackNameFromRecord pins the wiring half of #737: session,
// status, and destroy resolution must target the recorded stack instead of
// the ludus-<fleet-name> default whenever a stack deployment is recorded.
func TestResolveTarget_StackNameFromRecord(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
		want  string
	}{
		{
			name:  "recorded custom stack name wins over default",
			setup: func(t *testing.T) { recordDeployment(t, "stack", "custom-stack") },
			want:  "custom-stack",
		},
		{
			name:  "no record falls back to fleet-name default",
			setup: func(*testing.T) {},
			want:  "ludus-ludus-fleet",
		},
		{
			name:  "non-stack record falls back to fleet-name default",
			setup: func(t *testing.T) { recordDeployment(t, "ec2", "stale-stack") },
			want:  "ludus-ludus-fleet",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			tt.setup(t)

			if got := resolveStackTarget(t, stackCfg()).StackName; got != tt.want {
				t.Errorf("resolved StackName = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveTarget_StackForwardsMaxConcurrentSessions pins #743: the
// configured gamelift.maxConcurrentSessions must reach the stack options.
func TestResolveTarget_StackForwardsMaxConcurrentSessions(t *testing.T) {
	tests := []struct {
		name string
		mcs  int
		want int
	}{
		{name: "configured value forwarded", mcs: 3, want: 3},
		{name: "unset forwarded raw", mcs: 0, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			cfg := stackCfg()
			cfg.GameLift.MaxConcurrentSessions = tt.mcs
			if got := resolveStackTarget(t, cfg).MaxConcurrentSessions; got != tt.want {
				t.Errorf("resolved MaxConcurrentSessions = %d, want %d", got, tt.want)
			}
		})
	}
}
