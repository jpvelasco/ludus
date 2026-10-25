package gamelift

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/tags"
)

const (
	iamRoleName                = "LudusGameLiftContainerFleetRole"
	iamPolicyARN               = "arn:aws:iam::aws:policy/GameLiftContainerFleetPolicy"
	iamPropagationDelayDefault = 10 * time.Second
	pollInterval               = 15 * time.Second
	maxPollWait                = 30 * time.Minute

	// fleetNameTagKey is the ownership tag ludus writes on every container
	// fleet it creates. Fleets sharing a container group definition are
	// attributed to a deployment by this tag's value.
	fleetNameTagKey = "ludus:fleet-name"
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
		GameServerContainerGroupsPerInstance:   aws.Int32(int32(config.ResolvedMaxConcurrentSessions(opts.MaxConcurrentSessions))),
	}
}

// fleetCandidate is one container fleet sharing this deployer's container
// group definition, with its ludus fleet-name ownership tag resolved (empty
// when the fleet is untagged or its tags could not be read).
type fleetCandidate struct {
	fleetID   string
	status    string
	ownedName string
}

// findContainerFleetID returns the fleet this deployment's teardown must
// target. A recorded deployment fleet ID wins outright: deleting a fleet ID
// this profile created can never touch another deployment's fleet, and an
// already-deleted fleet is handled as not-found by the delete call itself.
// Without a recorded ID (legacy deployments), the fleet is discovered by
// ownership tag, falling back to a unique fleet, and refusing an arbitrary
// choice among several (#716).
func (d *Deployer) findContainerFleetID(ctx context.Context) (string, error) {
	if id := recordedFleetID(); id != "" {
		return id, nil
	}
	fleet, err := d.resolveFleet(ctx)
	if err != nil {
		return "", err
	}
	if fleet == nil {
		return "", nil
	}
	return fleet.fleetID, nil
}

// resolveFleet lists every fleet using this deployer's container group
// definition (following pagination) and selects the one owned by this
// deployment. The AWS calls stay thin here; the choice itself is selectFleet's.
func (d *Deployer) resolveFleet(ctx context.Context) (*fleetCandidate, error) {
	fleets, err := d.listContainerFleets(ctx)
	if err != nil {
		return nil, err
	}
	candidates := d.fleetCandidates(ctx, fleets)
	fleet, err := selectFleet(recordedFleetID(), d.opts.FleetName, candidates)
	if err != nil {
		return nil, fmt.Errorf("selecting fleet for container group %s: %w", d.opts.ContainerGroupName, err)
	}
	return fleet, nil
}

// listContainerFleets returns every fleet using this deployer's container
// group definition, following ListContainerFleets pagination to the last
// page so fleets beyond the first page are never missed. A not-found error
// yields no fleets.
func (d *Deployer) listContainerFleets(ctx context.Context) ([]gltypes.ContainerFleet, error) {
	var fleets []gltypes.ContainerFleet
	var nextToken *string
	for {
		out, err := d.glClient.ListContainerFleets(ctx, &gamelift.ListContainerFleetsInput{
			ContainerGroupDefinitionName: aws.String(d.opts.ContainerGroupName),
			NextToken:                    nextToken,
		})
		if err != nil {
			if awsutil.IsNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("listing fleets: %w", err)
		}
		fleets = append(fleets, out.ContainerFleets...)
		if aws.ToString(out.NextToken) == "" {
			return fleets, nil
		}
		nextToken = out.NextToken
	}
}

// fleetCandidates resolves the ownership tag of every listed fleet. Tag
// resolution is best-effort: a fleet whose tags cannot be read is treated as
// untagged, so a permissions hiccup degrades selection to uniqueness checks
// rather than failing the whole lookup.
func (d *Deployer) fleetCandidates(ctx context.Context, fleets []gltypes.ContainerFleet) []fleetCandidate {
	candidates := make([]fleetCandidate, 0, len(fleets))
	for i := range fleets {
		candidates = append(candidates, fleetCandidate{
			fleetID:   aws.ToString(fleets[i].FleetId),
			status:    string(fleets[i].Status),
			ownedName: d.fleetNameTag(ctx, fleets[i]),
		})
	}
	return candidates
}

// fleetNameTag resolves the ludus fleet-name ownership tag on one fleet,
// returning "" when the fleet has no ARN, the tag is absent, or the call
// fails.
func (d *Deployer) fleetNameTag(ctx context.Context, fleet gltypes.ContainerFleet) string {
	arn := aws.ToString(fleet.FleetArn)
	if arn == "" {
		return ""
	}
	out, err := d.glClient.ListTagsForResource(ctx, &gamelift.ListTagsForResourceInput{
		ResourceARN: aws.String(arn),
	})
	if err != nil || out == nil {
		return ""
	}
	for _, tag := range out.Tags {
		if aws.ToString(tag.Key) == fleetNameTagKey {
			return aws.ToString(tag.Value)
		}
	}
	return ""
}

// recordedFleetID returns the fleet ID recorded by this profile's most recent
// GameLift deployment, or "" when none is recorded. Fleet IDs recorded by
// other targets are ignored (Anywhere and stack deployments also write the
// fleet block), so this deployment never adopts — or destroys — a fleet it
// does not own.
func recordedFleetID() string {
	st, err := state.Load()
	if err != nil || st.Deploy == nil || st.Deploy.TargetName != "gamelift" || st.Fleet == nil {
		return ""
	}
	return st.Fleet.FleetID
}

// selectFleet picks the fleet owned by this deployment from the candidates
// sharing its container group definition. Precedence (#716):
//
//  1. the fleet matching the recorded deployment fleet ID — a recorded ID that
//     is absent from the account is an error, so a stale record can never
//     silently redirect status, sessions, or teardown onto another
//     deployment's fleet;
//  2. the unique fleet tagged ludus:fleet-name with this deployer's fleet name;
//  3. the only fleet sharing the container group;
//  4. never an arbitrary first match — ambiguity is an error.
//
// (nil, nil) means no fleet matches at all.
func selectFleet(recordedID, fleetName string, candidates []fleetCandidate) (*fleetCandidate, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	if recordedID != "" {
		if fleet := findCandidateByID(recordedID, candidates); fleet != nil {
			return fleet, nil
		}
		return nil, fmt.Errorf("recorded fleet %q not found among %d fleet(s): %s",
			recordedID, len(candidates), candidateIDs(candidates))
	}
	if fleet, ok := uniqueOwnedFleet(fleetName, candidates); ok {
		return fleet, nil
	}
	if len(candidates) == 1 {
		return &candidates[0], nil
	}
	return nil, fmt.Errorf("cannot identify this deployment's fleet: %d fleets share the container group and none is uniquely tagged %s=%q: %s",
		len(candidates), fleetNameTagKey, fleetName, candidateIDs(candidates))
}

func findCandidateByID(id string, candidates []fleetCandidate) *fleetCandidate {
	for i := range candidates {
		if candidates[i].fleetID == id {
			return &candidates[i]
		}
	}
	return nil
}

// uniqueOwnedFleet returns the only candidate owned by fleetName, or
// (nil, false) when zero or several candidates match.
func uniqueOwnedFleet(fleetName string, candidates []fleetCandidate) (*fleetCandidate, bool) {
	if fleetName == "" {
		return nil, false
	}
	var match *fleetCandidate
	for i := range candidates {
		if candidates[i].ownedName != fleetName {
			continue
		}
		if match != nil {
			return nil, false
		}
		match = &candidates[i]
	}
	return match, match != nil
}

func candidateIDs(candidates []fleetCandidate) string {
	ids := make([]string, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, c.fleetID)
	}
	return strings.Join(ids, ", ")
}
