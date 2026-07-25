package ec2fleet

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/state"
)

func TestTargetAdapter(t *testing.T) {
	d := NewDeployer(DeployOptions{FleetName: "f"}, aws.Config{}, runner.NewRunner(false, true))
	a := NewTargetAdapter(d)

	if a.Name() != "ec2" {
		t.Errorf("Name() = %q, want ec2", a.Name())
	}
	if a.Deployer() != d {
		t.Error("Deployer() did not return the wrapped deployer")
	}

	caps := a.Capabilities()
	if caps.NeedsContainerBuild || caps.NeedsContainerPush {
		t.Error("ec2 should not need container build/push")
	}
	if !caps.SupportsSession || !caps.SupportsDeploy || !caps.SupportsDestroy {
		t.Errorf("ec2 should support session/deploy/destroy, got %+v", caps)
	}
}

func TestDestroyStateFromState(t *testing.T) {
	tests := []struct {
		name  string
		state *state.State
		want  destroyState
	}{
		{"Nil destroyState", &state.State{EC2Fleet: nil}, destroyState{}},
		{"Populated destroyState",
			&state.State{EC2Fleet: &state.EC2FleetState{
				FleetID:  "Alpha-Fleet",
				BuildID:  "1",
				S3Bucket: "s3Bucket",
				S3Key:    "s3Key",
			}},
			destroyState{
				fleetID:  "Alpha-Fleet",
				buildID:  "1",
				s3Bucket: "s3Bucket",
				s3Key:    "s3Key",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := destroyStateFromState(tt.state)
			if got != tt.want {
				t.Errorf("Invalid DestroyState: got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDestroyState_Empty(t *testing.T) {
	tests := []struct {
		name   string
		dState destroyState
		want   bool
	}{
		{"Empty", destroyState{}, true},
		{"Populated", destroyState{fleetID: "Alpha-Fleet", buildID: "1"}, false},
		{"Partially populated", destroyState{fleetID: "Alpha-Fleet"}, false},
		{"Other fields populated", destroyState{s3Bucket: "s3Bucket", s3Key: "s3Key"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dState.empty()
			if got != tt.want {
				t.Errorf("Empty state mismatch: want %t, got %t", tt.want, got)
			}
		})
	}
}
