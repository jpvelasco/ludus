package gamelift

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/jpvelasco/ludus/internal/awsutil"
)

// containerFleetMaxWait is the wait window for container fleets specifically:
// large server images spend a long time in CREATED while GameLift pulls and
// snapshots them, which can exceed the generic 30-minute maxPollWait.
const containerFleetMaxWait = 60 * time.Minute

func (d *Deployer) waitForContainerFleetActive(ctx context.Context, fleetID string, result *FleetStatus) error {
	err := awsutil.Poll(ctx, pollInterval, containerFleetMaxWait, func() (bool, error) {
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
		// EXPIRED is terminal provisioning failure (e.g. bad image reference):
		// polling can never succeed, so fail fast with the actionable status.
		if status == gltypes.ContainerFleetStatusExpired {
			return false, fmt.Errorf("container fleet provisioning failed with status EXPIRED")
		}
		// While the fleet sits in CREATED/ACTIVATING, a deployment that has
		// gone IMPAIRED can never recover — fail fast with its id (#606).
		if err := d.failIfDeploymentImpaired(ctx, fleetID); err != nil {
			return false, err
		}
		return false, nil
	})
	return awsutil.WrapTimeout(err, "fleet to become ACTIVE")
}

// failIfDeploymentImpaired returns an error when the fleet's latest managed
// deployment reports IMPAIRED (image pull or startup failure on the instance).
func (d *Deployer) failIfDeploymentImpaired(ctx context.Context, fleetID string) error {
	depID := d.latestDeploymentID
	if depID == "" {
		desc, err := d.glClient.DescribeContainerFleet(ctx, &gamelift.DescribeContainerFleetInput{
			FleetId: aws.String(fleetID),
		})
		if err != nil {
			return nil // best-effort: fall back to plain polling on lookup failure
		}
		if desc.ContainerFleet == nil || desc.ContainerFleet.DeploymentDetails == nil {
			return nil // best-effort: fall back to plain polling on lookup failure
		}
		depID = aws.ToString(desc.ContainerFleet.DeploymentDetails.LatestDeploymentId)
		if depID == "" {
			return nil
		}
		d.latestDeploymentID = depID
	}

	out, err := d.glClient.DescribeFleetDeployment(ctx, &gamelift.DescribeFleetDeploymentInput{
		FleetId:      aws.String(fleetID),
		DeploymentId: aws.String(depID),
	})
	if err != nil {
		return nil // best-effort
	}
	if out.FleetDeployment != nil && out.FleetDeployment.DeploymentStatus == gltypes.DeploymentStatusImpaired {
		return fmt.Errorf("container fleet deployment %s is IMPAIRED — the image likely failed to pull or start; "+
			"check the fleet's CloudWatch logs and ECR image size", depID)
	}
	return nil
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
