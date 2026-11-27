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

// DeployAction reports what DeployFleet did to the deployment's fleet.
type DeployAction string

const (
	// ActionCreated means a new container fleet was created.
	ActionCreated DeployAction = "created"
	// ActionUpdated means the existing fleet was rolled onto a new image.
	ActionUpdated DeployAction = "updated"
	// ActionUnchanged means the fleet already ran the desired image.
	ActionUnchanged DeployAction = "unchanged"
)

// DeployFleet brings this deployment's container fleet in line with the
// configured image. With no fleet it creates one. With a fleet it rolls the
// fleet onto a new container group definition version in place, or does
// nothing when the fleet already runs the desired image. force cuts a new
// version even when nothing appears to have changed.
//
// Only a fleet this deployment owns is ever updated (see selectFleetForDeploy).
func (d *Deployer) DeployFleet(ctx context.Context, force bool) (*FleetStatus, error) {
	if err := d.validateOptions(); err != nil {
		return nil, err
	}
	candidates, err := d.fleetsReferencingCGD(ctx)
	if err != nil {
		return nil, err
	}
	fleet, err := selectFleetForDeploy(recordedFleetID(), d.opts.FleetName, candidates)
	if err != nil {
		return nil, err
	}
	if fleet == nil {
		return d.createNewFleet(ctx)
	}
	if others := cgdSurvivors(fleet.fleetID, candidates); len(others) > 0 {
		return nil, fmt.Errorf("container group definition %s is also used by %s, so changing it could roll another deployment's fleet; "+
			"give each deployment its own gamelift.containerGroupName", d.opts.ContainerGroupName, cgdSurvivorSummary(others))
	}
	return d.updateFleet(ctx, fleet.fleetID, force)
}

// createNewFleet is the first-deploy path: definition, then fleet.
func (d *Deployer) createNewFleet(ctx context.Context) (*FleetStatus, error) {
	fmt.Fprintln(d.output(), "Creating container group definition...")
	cgdARN, err := d.CreateContainerGroupDefinition(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(d.output(), "Container group definition ready: %s\n\n", cgdARN)

	fmt.Fprintln(d.output(), "Creating container fleet...")
	status, err := d.CreateFleet(ctx, cgdARN)
	if err != nil {
		return nil, err
	}
	status.Action = ActionCreated
	return status, nil
}

// fleetInspection is everything updateFleet needs to decide what to do.
type fleetInspection struct {
	fleet   *gltypes.ContainerFleet
	running *gltypes.ContainerGroupDefinition // the version the fleet is on
	latest  *gltypes.ContainerGroupDefinition // the newest version
	digest  string                            // current ECR digest of the image tag, "" when unreadable
}

// updateFleet rolls an existing fleet onto the desired image, or reports that
// it already runs it.
func (d *Deployer) updateFleet(ctx context.Context, fleetID string, force bool) (*FleetStatus, error) {
	in, err := d.inspectFleet(ctx, fleetID)
	if err != nil {
		return nil, err
	}
	decision := chooseUpdate(in.running, in.latest, d.containerGroupDefinitionInput(), in.digest, force)

	result := &FleetStatus{
		FleetID:              fleetID,
		FleetName:            d.opts.FleetName,
		Status:               string(in.fleet.Status),
		InstanceType:         aws.ToString(in.fleet.InstanceType),
		ContainerGroupDefARN: aws.ToString(in.running.ContainerGroupDefinitionArn),
	}
	if decision.action == updateNone {
		d.reportUnchanged(fleetID, decision.unverified)
		result.Action = ActionUnchanged
		return result, nil
	}

	fmt.Fprintf(d.output(), "Updating fleet %s in place: %s.\n", fleetID, decision.reason)
	cgdARN, err := d.definitionForRollout(ctx, decision, in.latest)
	if err != nil {
		return nil, err
	}
	result.ContainerGroupDefARN = cgdARN
	if err := d.rollFleet(ctx, fleetID, latestDeploymentID(in.fleet), cgdARN, result); err != nil {
		return nil, err
	}
	result.Action = ActionUpdated
	return result, nil
}

// inspectFleet gathers the fleet, the definition version it runs, the newest
// version, and the image's current digest. An unreadable digest is not fatal:
// the decision degrades to comparing URIs and says so.
func (d *Deployer) inspectFleet(ctx context.Context, fleetID string) (*fleetInspection, error) {
	fleet, err := d.describeFleet(ctx, fleetID)
	if err != nil {
		return nil, err
	}
	if err := fleetUpdatable(fleetID, fleet.Status); err != nil {
		return nil, err
	}
	d.warnInstanceTypeDrift(fleet)

	ref := aws.ToString(fleet.GameServerContainerGroupDefinitionArn)
	if ref == "" {
		ref = d.opts.ContainerGroupName
	}
	running, err := d.describeDefinition(ctx, ref)
	if err != nil {
		return nil, err
	}
	latest, err := d.describeDefinition(ctx, d.opts.ContainerGroupName)
	if err != nil {
		return nil, err
	}
	digest, digestErr := d.desiredImageDigest(ctx)
	if digestErr != nil {
		fmt.Fprintf(d.output(), "Warning: cannot read the image digest (%v); a re-pushed tag will not be detected, pass --force to redeploy.\n", digestErr)
	}
	return &fleetInspection{fleet: fleet, running: running, latest: latest, digest: digest}, nil
}

// fleetUpdatable refuses to update a fleet that is not ACTIVE: a second update
// would silently cancel a deployment already in progress.
func fleetUpdatable(fleetID string, status gltypes.ContainerFleetStatus) error {
	switch status {
	case gltypes.ContainerFleetStatusActive:
		return nil
	case gltypes.ContainerFleetStatusExpired:
		return fmt.Errorf("fleet %s is EXPIRED (provisioning failed); run 'ludus deploy destroy' and redeploy", fleetID)
	case gltypes.ContainerFleetStatusDeleting:
		return fmt.Errorf("fleet %s is being deleted; wait for the deletion to finish, then redeploy", fleetID)
	}
	return fmt.Errorf("fleet %s is %s; wait for it to become ACTIVE before deploying again (a new update would cancel the deployment in progress)",
		fleetID, status)
}

// warnInstanceTypeDrift tells the operator when the configured instance type
// differs from the live fleet's, since a fleet's instance type cannot change.
func (d *Deployer) warnInstanceTypeDrift(fleet *gltypes.ContainerFleet) {
	live := aws.ToString(fleet.InstanceType)
	if live == "" || d.opts.InstanceType == "" || live == d.opts.InstanceType {
		return
	}
	fmt.Fprintf(d.output(), "Warning: the fleet runs on %s but gamelift.instanceType is %s; an instance type cannot be changed in place. "+
		"Run 'ludus deploy destroy' and redeploy to apply it.\n", live, d.opts.InstanceType)
}

// reportUnchanged prints the no-op outcome, with the digest caveat when the
// comparison could not be made.
func (d *Deployer) reportUnchanged(fleetID string, unverified bool) {
	fmt.Fprintf(d.output(), "Fleet %s already runs the desired container group definition; nothing to deploy.\n", fleetID)
	if unverified {
		fmt.Fprintln(d.output(), "Note: the image digest could not be compared, so a re-pushed tag would not be noticed; pass --force to redeploy anyway.")
	}
}

// describeFleet fetches one container fleet.
func (d *Deployer) describeFleet(ctx context.Context, fleetID string) (*gltypes.ContainerFleet, error) {
	out, err := d.glClient.DescribeContainerFleet(ctx, &gamelift.DescribeContainerFleetInput{FleetId: aws.String(fleetID)})
	if err != nil {
		return nil, fmt.Errorf("describing fleet %s: %w", fleetID, err)
	}
	if out == nil || out.ContainerFleet == nil {
		return nil, fmt.Errorf("describing fleet %s: empty response", fleetID)
	}
	return out.ContainerFleet, nil
}

// describeDefinition fetches a container group definition by name (latest
// version) or by version-specific ARN.
func (d *Deployer) describeDefinition(ctx context.Context, ref string) (*gltypes.ContainerGroupDefinition, error) {
	out, err := d.cgdClient.DescribeContainerGroupDefinition(ctx, &gamelift.DescribeContainerGroupDefinitionInput{Name: aws.String(ref)})
	if err != nil {
		return nil, fmt.Errorf("describing container group definition %s: %w", ref, err)
	}
	if out == nil || out.ContainerGroupDefinition == nil {
		return nil, fmt.Errorf("describing container group definition %s: empty response", ref)
	}
	return out.ContainerGroupDefinition, nil
}

// definitionForRollout returns the version-specific ARN the fleet should move
// to: the latest version as is, or a new version cut from it. It returns once
// that version is READY.
func (d *Deployer) definitionForRollout(ctx context.Context, decision updateDecision, latest *gltypes.ContainerGroupDefinition) (string, error) {
	if decision.action == updateRollOnly {
		arn := aws.ToString(latest.ContainerGroupDefinitionArn)
		return arn, d.waitForContainerGroupReady(ctx, arn)
	}
	return d.createDefinitionVersion(ctx, latest)
}

// createDefinitionVersion cuts a new version of the group's definition and
// waits for GameLift to snapshot the image into it.
func (d *Deployer) createDefinitionVersion(ctx context.Context, latest *gltypes.ContainerGroupDefinition) (string, error) {
	fmt.Fprintln(d.output(), "Creating container group definition version...")
	description := "ludus deploy fleet " + time.Now().UTC().Format(time.RFC3339)
	out, err := d.cgdClient.UpdateContainerGroupDefinition(ctx, updateContainerGroupDefinitionInput(d.containerGroupDefinitionInput(), latest, description))
	if err != nil {
		return "", fmt.Errorf("creating container group definition version: %w", err)
	}
	if out == nil || out.ContainerGroupDefinition == nil || aws.ToString(out.ContainerGroupDefinition.ContainerGroupDefinitionArn) == "" {
		return "", fmt.Errorf("creating container group definition version: missing definition ARN in response")
	}
	arn := aws.ToString(out.ContainerGroupDefinition.ContainerGroupDefinitionArn)
	if err := d.waitForContainerGroupReady(ctx, arn); err != nil {
		return "", err
	}
	fmt.Fprintf(d.output(), "Container group definition version ready: %s\n", arn)
	return arn, nil
}

// rollFleet points the fleet at the definition version and waits for the
// deployment it starts to finish. prevDeploymentID is the fleet's deployment
// from before any change, so only a newer deployment counts as this rollout.
func (d *Deployer) rollFleet(ctx context.Context, fleetID, prevDeploymentID, cgdARN string, result *FleetStatus) error {
	fmt.Fprintf(d.output(), "Rolling fleet %s onto %s...\n", fleetID, cgdARN)
	if _, err := d.glClient.UpdateContainerFleet(ctx, updateContainerFleetInput(fleetID, cgdARN)); err != nil {
		return fmt.Errorf("updating container fleet: %w", err)
	}
	tracker := &rolloutTracker{prevDeploymentID: prevDeploymentID}
	err := awsutil.Poll(ctx, pollInterval, containerFleetMaxWait, func() (bool, error) {
		snap, err := d.snapshotRollout(ctx, fleetID, prevDeploymentID)
		if err != nil {
			return false, err
		}
		result.Status = string(snap.fleetStatus)
		fmt.Fprintf(d.output(), "  Fleet status: %s, deployment: %s\n", snap.fleetStatus, deploymentLabel(snap))
		return tracker.observe(snap)
	})
	return awsutil.WrapTimeout(err, "fleet deployment to complete")
}

// deploymentLabel names the deployment state for progress output.
func deploymentLabel(s fleetDeploymentSnapshot) string {
	if s.deployStatus == "" {
		return "not started"
	}
	return string(s.deployStatus)
}
