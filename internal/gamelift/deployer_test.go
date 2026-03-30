package gamelift

import (
	"testing"
)

func TestBuildCreateFleetInput_OmitsPortConfig(t *testing.T) {
	opts := DeployOptions{
		InstanceType:       "c5.large",
		ContainerGroupName: "test-group",
		ServerPort:         7777,
		Tags:               map[string]string{"ManagedBy": "ludus"},
	}
	resourceTags := map[string]string{"ManagedBy": "ludus", "ludus:fleet-name": "test-fleet"}

	input := buildCreateFleetInput(opts, "arn:aws:iam::123456789012:role/TestRole", resourceTags)

	// GameLift auto-calculates the optimal public port range when these are omitted.
	// Setting them manually risks hitting restricted ports (4080, 5757).
	if input.InstanceInboundPermissions != nil {
		t.Error("InstanceInboundPermissions must be nil — GameLift auto-calculates the port range")
	}
	if input.InstanceConnectionPortRange != nil {
		t.Error("InstanceConnectionPortRange must be nil — GameLift auto-calculates the port range")
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
}
