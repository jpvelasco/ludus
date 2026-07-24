package ec2fleet

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

func TestCreateFleetInput(t *testing.T) {
	d := &Deployer{opts: DeployOptions{
		FleetName:    "testing-fleet",
		InstanceType: "Test-Instance",
		ServerPort:   8000,
	}}
	buildId := "1"
	roleArn := "testingARN"

	got := d.createFleetInput(buildId, roleArn)

	checks := []struct{ field, want, got string }{
		{"Name", "testing-fleet", aws.ToString(got.Name)},
		{"Desciption", "Ludus dedicated server EC2 fleet", aws.ToString(got.Description)},
		{"BuildID", "1", aws.ToString(got.BuildId)},
		{"EC2 Type", "Test-Instance", string(got.EC2InstanceType)},
		{"FleetType", "ON_DEMAND", string(got.FleetType)},
		{"RoleArn", "testingARN", aws.ToString(got.InstanceRoleArn)},
		// Ignoring RuntimeConfiguration check as handled in another test function
		{"EC2InboundPerm: FromPort", string(int32(8000)), string(aws.ToInt32(got.EC2InboundPermissions[0].FromPort))},
		{"EC2InboundPerm: ToPort", string(int32(8000)), string(aws.ToInt32(got.EC2InboundPermissions[0].FromPort))},
		{"EC2InboundPerm: IpRange", "0.0.0.0/0", aws.ToString(got.EC2InboundPermissions[0].IpRange)},
		{"EC2InboundPerm: Protocol", "UDP", string(got.EC2InboundPermissions[0].Protocol)},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s Mismatch - got %s, want %s", c.field, c.got, c.want)
		}
	}
	// Check of GameLiftTags
	tagMap := make(map[string]string)
	for _, tag := range got.Tags {
		tagMap[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	if tagMap["ludus:fleet-name"] != "testing-fleet" || tagMap["ludus:target"] != "ec2" {
		t.Errorf("unexpected tags wired to CreateBuildInput: %v", tagMap)
	}
}

func TestFleetActivePollResult(t *testing.T) {
	tests := []struct {
		name     string
		status   gltypes.FleetStatus
		wantBool bool
		wantErr  error
	}{
		{"FleetStatusActive", gltypes.FleetStatusActive, true, nil},
		{"FleetStatusError", gltypes.FleetStatusError, false, errors.New("fleet entered ERROR state")},
		{"FleetStatusOther", gltypes.FleetStatusBuilding, false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBool, gotErr := fleetActivePollResult(tt.status)
			if tt.wantBool != gotBool || (gotErr != nil && tt.wantErr.Error() != gotErr.Error()) {
				t.Errorf("Got bool: %t, err: %v; Want bool %v, err %v.", gotBool, gotErr, tt.wantBool, tt.wantErr)
			}
		})
	}
}

func TestRuntimeConfiguration(t *testing.T) {
	d := &Deployer{opts: DeployOptions{ServerPort: 8000}}
	got := d.runtimeConfiguration()

	checks := []struct {
		field, want, got string
	}{
		{"LaunchPath", "/local/game/amazon-gamelift-servers-game-server-wrapper", aws.ToString(got.ServerProcesses[0].LaunchPath)},
		{"Parameters", "--port 8000", aws.ToString(got.ServerProcesses[0].Parameters)},
		{"ConcurrentExecutions", string(int32(1)), string(aws.ToInt32(got.ServerProcesses[0].ConcurrentExecutions))},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s Mismatch - got %s, want %s", c.field, c.got, c.want)
		}
	}
}
