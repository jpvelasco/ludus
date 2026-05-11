package gamelift

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/devrecon/ludus/internal/awsutil"
)

func (d *Deployer) waitForContainerFleetActive(ctx context.Context, fleetID string, result *FleetStatus) error {
	err := awsutil.Poll(ctx, pollInterval, maxPollWait, func() (bool, error) {
		desc, err := d.glClient.DescribeContainerFleet(ctx, &gamelift.DescribeContainerFleetInput{
			FleetId: aws.String(fleetID),
		})
		if err != nil {
			return false, fmt.Errorf("polling fleet status: %w", err)
		}

		status := desc.ContainerFleet.Status
		result.Status = string(status)
		fmt.Printf("  Fleet status: %s\n", status)

		if status == gltypes.ContainerFleetStatusActive {
			return true, nil
		}
		return false, nil
	})
	return awsutil.WrapTimeout(err, "fleet to become ACTIVE")
}

// GetFleetStatus returns the current status of the deployed fleet.
func (d *Deployer) GetFleetStatus(ctx context.Context) (*FleetStatus, error) {
	out, err := d.glClient.ListContainerFleets(ctx, &gamelift.ListContainerFleetsInput{
		ContainerGroupDefinitionName: aws.String(d.opts.ContainerGroupName),
	})
	if err != nil {
		return nil, fmt.Errorf("listing fleets: %w", err)
	}

	if len(out.ContainerFleets) == 0 {
		return nil, fmt.Errorf("no fleets found for container group %s", d.opts.ContainerGroupName)
	}

	fleet := out.ContainerFleets[0]
	return &FleetStatus{
		FleetID: aws.ToString(fleet.FleetId),
		Status:  string(fleet.Status),
	}, nil
}

func (d *Deployer) deleteFleet(ctx context.Context) error {
	fmt.Println("Deleting fleet...")

	fleetID, err := d.findContainerFleetID(ctx)
	if err != nil {
		return err
	}
	if fleetID == "" {
		fmt.Println("No fleet found, skipping.")
		return nil
	}

	if err := d.deleteContainerFleet(ctx, fleetID); err != nil {
		return err
	}
	return d.waitForContainerFleetDeletion(ctx, fleetID)
}

func (d *Deployer) deleteContainerFleet(ctx context.Context, fleetID string) error {
	_, err := d.glClient.DeleteContainerFleet(ctx, &gamelift.DeleteContainerFleetInput{
		FleetId: aws.String(fleetID),
	})
	if err == nil {
		return nil
	}
	if awsutil.IsNotFound(err) {
		fmt.Println("Fleet already deleted.")
		return nil
	}
	return fmt.Errorf("deleting fleet: %w", err)
}

func (d *Deployer) waitForContainerFleetDeletion(ctx context.Context, fleetID string) error {
	err := awsutil.Poll(ctx, pollInterval, maxPollWait, func() (bool, error) {
		_, err := d.glClient.DescribeContainerFleet(ctx, &gamelift.DescribeContainerFleetInput{
			FleetId: aws.String(fleetID),
		})
		if err != nil {
			if awsutil.IsNotFound(err) {
				fmt.Println("Fleet deleted.")
				return true, nil
			}
			return false, fmt.Errorf("polling fleet deletion: %w", err)
		}
		fmt.Println("  Waiting for fleet deletion...")
		return false, nil
	})
	return awsutil.WrapTimeout(err, "fleet deletion")
}
