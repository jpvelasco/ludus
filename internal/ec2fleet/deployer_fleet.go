package ec2fleet

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/devrecon/ludus/internal/awsutil"
	"github.com/devrecon/ludus/internal/tags"
)

func (d *Deployer) createFleetInput(buildID, roleARN string) *gamelift.CreateFleetInput {
	return &gamelift.CreateFleetInput{
		Name:                 aws.String(d.opts.FleetName),
		Description:          aws.String("Ludus dedicated server EC2 fleet"),
		BuildId:              aws.String(buildID),
		EC2InstanceType:      gltypes.EC2InstanceType(d.opts.InstanceType),
		FleetType:            gltypes.FleetTypeOnDemand,
		InstanceRoleArn:      aws.String(roleARN),
		RuntimeConfiguration: d.runtimeConfiguration(),
		EC2InboundPermissions: []gltypes.IpPermission{
			{
				FromPort: aws.Int32(int32(d.opts.ServerPort)),
				ToPort:   aws.Int32(int32(d.opts.ServerPort)),
				IpRange:  aws.String("0.0.0.0/0"),
				Protocol: gltypes.IpProtocolUdp,
			},
		},
		Tags: tags.ToGameLiftTags(d.resourceTags()),
	}
}

func (d *Deployer) runtimeConfiguration() *gltypes.RuntimeConfiguration {
	// The wrapper binary is at the root of the zip; game server details
	// are configured in config.yaml. The wrapper accepts --port only.
	launchPath := "/local/game/amazon-gamelift-servers-game-server-wrapper"
	launchParams := fmt.Sprintf("--port %d", d.opts.ServerPort)

	return &gltypes.RuntimeConfiguration{
		ServerProcesses: []gltypes.ServerProcess{
			{
				LaunchPath:           aws.String(launchPath),
				Parameters:           aws.String(launchParams),
				ConcurrentExecutions: aws.Int32(1),
			},
		},
	}
}

func (d *Deployer) waitForFleetActive(ctx context.Context, fleetID string, result *FleetStatus) error {
	err := awsutil.Poll(ctx, pollInterval, maxPollWait, func() (bool, error) {
		return d.pollFleetActiveStatus(ctx, fleetID, result)
	})
	if err != nil && !errors.Is(err, awsutil.ErrPollTimeout) {
		return err
	}
	if errors.Is(err, awsutil.ErrPollTimeout) {
		return fmt.Errorf("timed out waiting for fleet to become ACTIVE")
	}
	return nil
}

func (d *Deployer) pollFleetActiveStatus(ctx context.Context, fleetID string, result *FleetStatus) (bool, error) {
	desc, err := d.glClient.DescribeFleetAttributes(ctx, &gamelift.DescribeFleetAttributesInput{
		FleetIds: []string{fleetID},
	})
	if err != nil {
		return false, fmt.Errorf("polling fleet status: %w", err)
	}
	if len(desc.FleetAttributes) == 0 {
		return false, fmt.Errorf("fleet %s disappeared during polling", fleetID)
	}

	status := desc.FleetAttributes[0].Status
	result.Status = string(status)
	fmt.Printf("  Fleet status: %s\n", status)

	return fleetActivePollResult(status)
}

func fleetActivePollResult(status gltypes.FleetStatus) (bool, error) {
	if status == gltypes.FleetStatusActive {
		return true, nil
	}
	if status == gltypes.FleetStatusError {
		return false, fmt.Errorf("fleet entered ERROR state")
	}
	return false, nil
}

// deleteFleetResource deletes the fleet and polls until it is gone.
func (d *Deployer) deleteFleetResource(ctx context.Context, fleetID string) error {
	if fleetID == "" {
		return nil
	}

	fmt.Println("Deleting fleet...")
	_, err := d.glClient.DeleteFleet(ctx, &gamelift.DeleteFleetInput{
		FleetId: aws.String(fleetID),
	})
	if err != nil && !awsutil.IsNotFound(err) {
		return fmt.Errorf("deleting fleet: %w", err)
	}

	if err := d.waitForFleetDeletion(ctx, fleetID); err != nil {
		return err
	}
	fmt.Println("Fleet deleted.")
	return nil
}

// waitForFleetDeletion polls until the fleet no longer exists.
func (d *Deployer) waitForFleetDeletion(ctx context.Context, fleetID string) error {
	err := awsutil.Poll(ctx, pollInterval, maxPollWait, func() (bool, error) {
		desc, err := d.glClient.DescribeFleetAttributes(ctx, &gamelift.DescribeFleetAttributesInput{
			FleetIds: []string{fleetID},
		})
		if err != nil {
			if awsutil.IsNotFound(err) {
				return true, nil
			}
			return false, fmt.Errorf("polling fleet deletion: %w", err)
		}
		if len(desc.FleetAttributes) == 0 {
			return true, nil
		}
		fmt.Println("  Waiting for fleet deletion...")
		return false, nil
	})
	if errors.Is(err, awsutil.ErrPollTimeout) {
		return nil
	}
	return err
}

// deleteBuildResource deletes the GameLift build, logging a warning on failure.
func (d *Deployer) deleteBuildResource(ctx context.Context, buildID string) {
	if buildID == "" {
		return
	}

	fmt.Println("Deleting build...")
	_, err := d.glClient.DeleteBuild(ctx, &gamelift.DeleteBuildInput{
		BuildId: aws.String(buildID),
	})
	if err != nil && !awsutil.IsNotFound(err) {
		fmt.Printf("Warning: failed to delete build: %v\n", err)
		return
	}
	fmt.Println("Build deleted.")
}
