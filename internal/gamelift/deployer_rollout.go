package gamelift

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

// maxIdleRolloutPolls is how many consecutive polls an ACTIVE fleet may still
// show its previous deployment before the update is judged to have started
// none. A short lag after UpdateContainerFleet is normal; an update that
// changed nothing starts no deployment at all.
const maxIdleRolloutPolls = 8

// updateContainerGroupDefinitionInput builds the request that cuts a new
// version of the group's definition. It starts from the latest version and
// replaces the game server container, limits and ports with the desired ones,
// so the create and update paths share one source for those values.
func updateContainerGroupDefinitionInput(desired *gamelift.CreateContainerGroupDefinitionInput, latest *gltypes.ContainerGroupDefinition, description string) *gamelift.UpdateContainerGroupDefinitionInput {
	return &gamelift.UpdateContainerGroupDefinitionInput{
		Name:                          desired.Name,
		GameServerContainerDefinition: desired.GameServerContainerDefinition,
		TotalMemoryLimitMebibytes:     desired.TotalMemoryLimitMebibytes,
		TotalVcpuLimit:                desired.TotalVcpuLimit,
		SourceVersionNumber:           latest.VersionNumber,
		VersionDescription:            aws.String(description),
	}
}

// updateContainerFleetInput points the fleet at one specific definition
// version. A failed validation rolls back to the previous version so a bad
// image leaves the live fleet serving the old one, and WITH_PROTECTION keeps
// protected game sessions alive during the replacement.
func updateContainerFleetInput(fleetID, definitionARN string) *gamelift.UpdateContainerFleetInput {
	return &gamelift.UpdateContainerFleetInput{
		FleetId:                                aws.String(fleetID),
		GameServerContainerGroupDefinitionName: aws.String(definitionARN),
		DeploymentConfiguration: &gltypes.DeploymentConfiguration{
			ProtectionStrategy: gltypes.DeploymentProtectionStrategyWithProtection,
			ImpairmentStrategy: gltypes.DeploymentImpairmentStrategyRollback,
		},
	}
}

// fleetDeploymentSnapshot is what one poll of a rolling fleet observed.
type fleetDeploymentSnapshot struct {
	fleetStatus  gltypes.ContainerFleetStatus
	deploymentID string
	deployStatus gltypes.DeploymentStatus
}

// rolloutTracker decides, poll by poll, whether the deployment started by an
// update has finished. Only a deployment other than the one the fleet had
// before the update counts: right after UpdateContainerFleet the fleet can
// still report its previous, COMPLETE deployment.
type rolloutTracker struct {
	prevDeploymentID string
	idlePolls        int
}

// observe returns done=true once the new deployment is COMPLETE and the fleet
// is ACTIVE again, and an error as soon as the deployment can no longer
// succeed.
func (t *rolloutTracker) observe(s fleetDeploymentSnapshot) (bool, error) {
	if s.fleetStatus == gltypes.ContainerFleetStatusExpired {
		return false, fmt.Errorf("container fleet became EXPIRED during the update; run 'ludus deploy destroy' and redeploy")
	}
	if s.deploymentID == "" || s.deploymentID == t.prevDeploymentID {
		return false, t.awaitDeploymentStart(s.fleetStatus)
	}
	switch s.deployStatus {
	case gltypes.DeploymentStatusComplete:
		return s.fleetStatus == gltypes.ContainerFleetStatusActive, nil
	case gltypes.DeploymentStatusImpaired,
		gltypes.DeploymentStatusRollbackInProgress,
		gltypes.DeploymentStatusRollbackComplete,
		gltypes.DeploymentStatusCancelled:
		return false, deploymentFailure(s.deploymentID, s.deployStatus)
	}
	return false, nil
}

// awaitDeploymentStart tolerates the lag before a new deployment registers,
// but not forever on an ACTIVE fleet: that means the update started none.
func (t *rolloutTracker) awaitDeploymentStart(fleetStatus gltypes.ContainerFleetStatus) error {
	if fleetStatus != gltypes.ContainerFleetStatusActive {
		t.idlePolls = 0
		return nil
	}
	t.idlePolls++
	if t.idlePolls >= maxIdleRolloutPolls {
		return fmt.Errorf("the fleet update returned but no new deployment started and the fleet is ACTIVE; " +
			"GameLift saw no change to the containers it runs — re-run with --force to cut a new definition version")
	}
	return nil
}

// deploymentFailure explains a deployment that can no longer succeed and what
// to check next.
func deploymentFailure(deploymentID string, status gltypes.DeploymentStatus) error {
	const check = "check the fleet events (GAME_SERVER_CONTAINER_GROUP_CRASHED) and the container logs in CloudWatch, fix the image, then redeploy"
	switch status {
	case gltypes.DeploymentStatusRollbackInProgress, gltypes.DeploymentStatusRollbackComplete:
		return fmt.Errorf("container fleet deployment %s failed validation (status %s) and GameLift is returning the fleet to its previous image; "+
			"the new image likely failed to pull or start: %s", deploymentID, status, check)
	case gltypes.DeploymentStatusCancelled:
		return fmt.Errorf("container fleet deployment %s was cancelled by a newer update to this fleet; "+
			"find out what else updated it, then redeploy", deploymentID)
	}
	return fmt.Errorf("container fleet deployment %s is IMPAIRED: the new containers never reported ready and were kept for inspection; %s", deploymentID, check)
}

// snapshotRollout reads the fleet and, once the fleet reports a deployment
// other than prevDeploymentID, that deployment's status.
func (d *Deployer) snapshotRollout(ctx context.Context, fleetID, prevDeploymentID string) (fleetDeploymentSnapshot, error) {
	fleet, err := d.describeFleet(ctx, fleetID)
	if err != nil {
		return fleetDeploymentSnapshot{}, fmt.Errorf("polling fleet status: %w", err)
	}
	snap := fleetDeploymentSnapshot{fleetStatus: fleet.Status, deploymentID: latestDeploymentID(fleet)}
	if snap.deploymentID == "" || snap.deploymentID == prevDeploymentID {
		return snap, nil
	}
	out, err := d.glClient.DescribeFleetDeployment(ctx, &gamelift.DescribeFleetDeploymentInput{
		FleetId:      aws.String(fleetID),
		DeploymentId: aws.String(snap.deploymentID),
	})
	if err != nil {
		return fleetDeploymentSnapshot{}, fmt.Errorf("polling fleet deployment %s: %w", snap.deploymentID, err)
	}
	if out != nil && out.FleetDeployment != nil {
		snap.deployStatus = out.FleetDeployment.DeploymentStatus
	}
	return snap, nil
}
