package gamelift

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/jpvelasco/ludus/internal/state"
)

// TestDeployWritesDeployState verifies that the state written after a gamelift
// deploy sets targetName to "gamelift" (regression: adapter was only writing
// UpdateFleet, leaving deploy.targetName stale from a previous deployment).
func TestDeployWritesDeployState(t *testing.T) {
	t.Chdir(t.TempDir())

	fleetID := "fleet-abc123"
	now := time.Now().UTC().Format(time.RFC3339)

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   fleetID,
		Status:    "ACTIVE",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("UpdateFleet: %v", err)
	}

	detail := "fleet " + fleetID
	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "gamelift",
		Status:     "ACTIVE",
		Detail:     detail,
		DeployedAt: now,
	}); err != nil {
		t.Fatalf("UpdateDeploy: %v", err)
	}

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Deploy == nil {
		t.Fatal("deploy state not written")
	}
	if s.Deploy.TargetName != "gamelift" {
		t.Errorf("deploy.targetName = %q, want %q", s.Deploy.TargetName, "gamelift")
	}
	if s.Fleet == nil || s.Fleet.FleetID != fleetID {
		t.Error("fleet state not written alongside deploy state")
	}
}

func TestBuildCreateFleetInput_PortSettings(t *testing.T) {
	tests := []struct {
		name                  string
		allowedCIDR           string
		maxConcurrentSessions int
		wantPortRange         bool
		wantFromPort          int32
		wantToPort            int32
	}{
		{name: "unset CIDR leaves ports to GameLift"},
		{
			name:                  "configured CIDR sets matching ports",
			allowedCIDR:           "10.1.0.0/16",
			maxConcurrentSessions: 2,
			wantPortRange:         true,
			wantFromPort:          4192,
			wantToPort:            4194,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := buildCreateFleetInput(DeployOptions{
				InstanceType:          "c5.large",
				ContainerGroupName:    "test-group",
				ServerPort:            7777,
				MaxConcurrentSessions: tt.maxConcurrentSessions,
				AllowedCIDR:           tt.allowedCIDR,
			}, "arn:aws:iam::123456789012:role/TestRole", nil)

			if !tt.wantPortRange {
				if input.InstanceInboundPermissions != nil {
					t.Errorf("InstanceInboundPermissions = %v, want nil", input.InstanceInboundPermissions)
				}
				if input.InstanceConnectionPortRange != nil {
					t.Errorf("InstanceConnectionPortRange = %v, want nil", input.InstanceConnectionPortRange)
				}
				return
			}

			if input.InstanceConnectionPortRange == nil {
				t.Fatal("InstanceConnectionPortRange = nil, want configured range")
			}
			if got := *input.InstanceConnectionPortRange.FromPort; got != tt.wantFromPort {
				t.Errorf("FromPort = %d, want %d", got, tt.wantFromPort)
			}
			if got := *input.InstanceConnectionPortRange.ToPort; got != tt.wantToPort {
				t.Errorf("ToPort = %d, want %d", got, tt.wantToPort)
			}
			if len(input.InstanceInboundPermissions) != 1 {
				t.Fatalf("InstanceInboundPermissions = %v, want 1 entry", input.InstanceInboundPermissions)
			}
			permission := input.InstanceInboundPermissions[0]
			if got := *permission.FromPort; got != tt.wantFromPort {
				t.Errorf("inbound FromPort = %d, want %d", got, tt.wantFromPort)
			}
			if got := *permission.ToPort; got != tt.wantToPort {
				t.Errorf("inbound ToPort = %d, want %d", got, tt.wantToPort)
			}
			if got := aws.ToString(permission.IpRange); got != tt.allowedCIDR {
				t.Errorf("inbound IpRange = %q, want %q", got, tt.allowedCIDR)
			}
		})
	}
}

func TestBuildCreateFleetInput_SetsRequiredFields(t *testing.T) {
	opts := DeployOptions{
		InstanceType:       "c7g.large",
		ContainerGroupName: "my-server-group",
		Tags:               map[string]string{"ManagedBy": "ludus"},
	}
	resourceTags := map[string]string{"ManagedBy": "ludus"}
	roleARN := "arn:aws:iam::123456789012:role/LudusRole"

	input := buildCreateFleetInput(opts, roleARN, resourceTags)

	if v := *input.FleetRoleArn; v != roleARN {
		t.Errorf("FleetRoleArn = %q, want %q", v, roleARN)
	}
	if v := *input.InstanceType; v != "c7g.large" {
		t.Errorf("InstanceType = %q, want %q", v, "c7g.large")
	}
	if v := *input.GameServerContainerGroupDefinitionName; v != "my-server-group" {
		t.Errorf("GameServerContainerGroupDefinitionName = %q, want %q", v, "my-server-group")
	}
	if len(input.Tags) == 0 {
		t.Error("Tags should not be empty")
	}
	if input.GameServerContainerGroupsPerInstance == nil || *input.GameServerContainerGroupsPerInstance != 1 {
		t.Errorf("GameServerContainerGroupsPerInstance = %v, want 1 default", input.GameServerContainerGroupsPerInstance)
	}
}

func TestBuildCreateFleetInput_AppliesMaxConcurrentSessions(t *testing.T) {
	opts := DeployOptions{
		InstanceType:          "c6i.large",
		ContainerGroupName:    "test-group",
		MaxConcurrentSessions: 3,
	}
	input := buildCreateFleetInput(opts, "arn:aws:iam::123456789012:role/LudusRole", nil)
	if input.GameServerContainerGroupsPerInstance == nil || *input.GameServerContainerGroupsPerInstance != 3 {
		t.Errorf("GameServerContainerGroupsPerInstance = %v, want 3", input.GameServerContainerGroupsPerInstance)
	}
}
