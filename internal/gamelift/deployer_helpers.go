package gamelift

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/tags"
)

const (
	iamRoleName  = "LudusGameLiftContainerFleetRole"
	iamPolicyARN = "arn:aws:iam::aws:policy/GameLiftContainerFleetPolicy"
	pollInterval = 15 * time.Second
	maxPollWait  = 30 * time.Minute
)

// buildCreateFleetInput constructs the CreateContainerFleetInput for GameLift.
// It deliberately omits InstanceInboundPermissions and InstanceConnectionPortRange
// so that GameLift auto-calculates the optimal public port range.
func buildCreateFleetInput(opts DeployOptions, roleARN string, resourceTags map[string]string) *gamelift.CreateContainerFleetInput {
	return &gamelift.CreateContainerFleetInput{
		FleetRoleArn:                           aws.String(roleARN),
		Description:                            aws.String("Ludus dedicated server fleet"),
		InstanceType:                           aws.String(opts.InstanceType),
		Tags:                                   tags.ToGameLiftTags(resourceTags),
		GameServerContainerGroupDefinitionName: aws.String(opts.ContainerGroupName),
	}
}

func (d *Deployer) findContainerFleetID(ctx context.Context) (string, error) {
	listOut, err := d.glClient.ListContainerFleets(ctx, &gamelift.ListContainerFleetsInput{
		ContainerGroupDefinitionName: aws.String(d.opts.ContainerGroupName),
	})
	if err != nil {
		if awsutil.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("listing fleets: %w", err)
	}
	if len(listOut.ContainerFleets) == 0 {
		return "", nil
	}
	return aws.ToString(listOut.ContainerFleets[0].FleetId), nil
}
