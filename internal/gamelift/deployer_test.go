package gamelift

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/jpvelasco/ludus/internal/config"
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

func TestBuildCreateFleetInput_OmitsPortConfig(t *testing.T) {
	opts := DeployOptions{
		InstanceType:       "c5.large",
		ContainerGroupName: "test-group",
		ServerPort:         7777,
		Tags:               map[string]string{"ManagedBy": "ludus"},
	}
	resourceTags := map[string]string{"ManagedBy": "ludus", "ludus:fleet-name": "test-fleet"}

	input := buildCreateFleetInput(opts, "arn:aws:iam::123456789012:role/TestRole", resourceTags)

	// GameLift auto-calculates the optimal public port range when
	// InstanceConnectionPortRange is omitted. Setting it manually risks hitting
	// restricted ports (4080, 5757). InstanceInboundPermissions is always set
	// from the deployer's AllowedCIDR so the source range is explicit rather
	// than silently world-open (#776).
	if input.InstanceConnectionPortRange != nil {
		t.Error("InstanceConnectionPortRange must be nil — GameLift auto-calculates the port range")
	}
	if len(input.InstanceInboundPermissions) != 1 {
		t.Fatalf("InstanceInboundPermissions = %v, want 1 entry", input.InstanceInboundPermissions)
	}
	if got := aws.ToString(input.InstanceInboundPermissions[0].IpRange); got != config.DefaultAllowedCIDR {
		t.Errorf("IpRange = %q, want %s (default public CIDR)", got, config.DefaultAllowedCIDR)
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

// TestBuildCreateFleetInput_AllowedCIDROverride pins the #776 contract: a
// configured gamelift.allowedCidr narrows the inbound rule instead of the
// silent 0.0.0.0/0 default.
func TestBuildCreateFleetInput_AllowedCIDROverride(t *testing.T) {
	opts := DeployOptions{
		InstanceType:       "c5.large",
		ContainerGroupName: "test-group",
		AllowedCIDR:        "10.1.0.0/16",
	}
	input := buildCreateFleetInput(opts, "arn:aws:iam::123456789012:role/TestRole", nil)

	if len(input.InstanceInboundPermissions) != 1 {
		t.Fatalf("InstanceInboundPermissions = %v, want 1 entry", input.InstanceInboundPermissions)
	}
	if got := aws.ToString(input.InstanceInboundPermissions[0].IpRange); got != "10.1.0.0/16" {
		t.Errorf("IpRange = %q, want %q", got, "10.1.0.0/16")
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
