package ec2fleet

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/tags"
)

// ec2WrapperLaunchPath is the Game Server Wrapper binary's location inside
// the GameLift EC2 build archive (the wrapper sits at the zip root; game
// server details are configured in config.yaml, but --port overrides its
// ports.gamePort per process).
const ec2WrapperLaunchPath = "/local/game/amazon-gamelift-servers-game-server-wrapper"

// maxServerProcesses is GameLift's per-runtime-configuration ServerProcesses
// limit. Each concurrent server process gets its own entry (and port), so the
// configured concurrency is clamped to it.
const maxServerProcesses = 50

func (d *Deployer) createFleetInput(buildID, roleARN string) *gamelift.CreateFleetInput {
	processCount := d.serverProcessCount()
	return &gamelift.CreateFleetInput{
		Name:                 aws.String(d.opts.FleetName),
		Description:          aws.String("Ludus dedicated server EC2 fleet"),
		BuildId:              aws.String(buildID),
		EC2InstanceType:      gltypes.EC2InstanceType(d.opts.InstanceType),
		FleetType:            gltypes.FleetTypeOnDemand,
		InstanceRoleArn:      aws.String(roleARN),
		RuntimeConfiguration: runtimeConfiguration(d.opts.ServerPort, processCount),
		EC2InboundPermissions: inboundPermissions(
			d.opts.AllowedCIDR, d.opts.ServerPort, processCount),
		Tags: tags.ToGameLiftTags(d.resourceTags()),
	}
}

// OpenCIDRWarning returns the public-CIDR warning for the fleet's UDP port,
// or "" when the inbound rule is restricted to a specific CIDR.
func OpenCIDRWarning(cidr string, serverPort int) string {
	if config.ResolvedAllowedCIDR(cidr) != config.DefaultAllowedCIDR {
		return ""
	}
	return fmt.Sprintf("WARNING: fleet UDP port %d will be open to 0.0.0.0/0 (public).\n"+
		"         Public UDP is often required for GameLift multiplayer, but set\n"+
		"         ec2fleet.allowedCidr in ludus.yaml to a tighter CIDR for\n"+
		"         private or partner-only deployments.", serverPort)
}

// serverProcessCount resolves how many concurrent server processes each
// instance runs: the configured maxConcurrentSessions, at least 1, clamped
// to GameLift's per-fleet ServerProcesses limit. Each process gets its own
// port (and RuntimeConfiguration entry), so the limit is on entries.
func (d *Deployer) serverProcessCount() int {
	count := config.ResolvedMaxConcurrentSessions(d.opts.MaxConcurrentSessions)
	if count <= maxServerProcesses {
		return count
	}
	fmt.Fprintf(d.output(), "Warning: maxConcurrentSessions %d exceeds GameLift's limit of %d server processes per fleet; clamping to %d\n",
		count, maxServerProcesses, maxServerProcesses)
	return maxServerProcesses
}

// serverProcessPorts returns the distinct UDP ports the concurrent server
// processes bind: [base, base+1, ..., base+count-1].
func serverProcessPorts(base, count int) []int {
	ports := make([]int, count)
	for i := range count {
		ports[i] = base + i
	}
	return ports
}

// runtimeConfiguration builds one ServerProcesses entry per concurrent server
// process, each pinned to a distinct --port with ConcurrentExecutions=1.
// A single entry with ConcurrentExecutions=N would launch N identical
// processes that all contend for the same socket; the wrapper's --port flag
// overrides its config.yaml gamePort, so the per-entry parameter gives every
// process its own endpoint.
func runtimeConfiguration(serverPort, count int) *gltypes.RuntimeConfiguration {
	processes := make([]gltypes.ServerProcess, 0, count)
	for _, port := range serverProcessPorts(serverPort, count) {
		processes = append(processes, gltypes.ServerProcess{
			LaunchPath:           aws.String(ec2WrapperLaunchPath),
			Parameters:           aws.String(fmt.Sprintf("--port %d", port)),
			ConcurrentExecutions: aws.Int32(1),
		})
	}
	return &gltypes.RuntimeConfiguration{ServerProcesses: processes}
}

// inboundPermissions opens UDP for the whole per-process port block. The
// ports are contiguous, so a single range rule [base, base+count-1] covers
// every server process.
func inboundPermissions(cidr string, serverPort, count int) []gltypes.IpPermission {
	return []gltypes.IpPermission{
		{
			FromPort: aws.Int32(int32(serverPort)),
			ToPort:   aws.Int32(int32(serverPort + count - 1)),
			IpRange:  aws.String(config.ResolvedAllowedCIDR(cidr)),
			Protocol: gltypes.IpProtocolUdp,
		},
	}
}

func (d *Deployer) waitForFleetActive(ctx context.Context, fleetID string, result *FleetStatus) error {
	err := awsutil.Poll(ctx, pollInterval, maxPollWait, func() (bool, error) {
		return d.pollFleetActiveStatus(ctx, fleetID, result)
	})
	return awsutil.WrapTimeout(err, "fleet to become ACTIVE")
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
	fmt.Fprintf(d.output(), "  Fleet status: %s\n", status)

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

	fmt.Fprintln(d.output(), "Deleting fleet...")
	_, err := d.glClient.DeleteFleet(ctx, &gamelift.DeleteFleetInput{
		FleetId: aws.String(fleetID),
	})
	if err != nil && !awsutil.IsNotFound(err) {
		return fmt.Errorf("deleting fleet: %w", err)
	}

	if err := d.waitForFleetDeletion(ctx, fleetID); err != nil {
		return err
	}
	fmt.Fprintln(d.output(), "Fleet deleted.")
	return nil
}

// waitForFleetDeletion polls until the fleet no longer exists. A poll
// timeout means the fleet still exists and must NOT be reported as deleted —
// the error keeps the deployment state intact for a safe retry.
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
		fmt.Fprintln(d.output(), "  Waiting for fleet deletion...")
		return false, nil
	})
	return fleetDeletionTimeout(err, fleetID)
}

// fleetDeletionTimeout converts a deletion-poll timeout into a contextual
// error so a fleet that outlives the wait is reported as still existing.
// Non-timeout poll results (including nil) pass through unchanged.
func fleetDeletionTimeout(err error, fleetID string) error {
	if !errors.Is(err, awsutil.ErrPollTimeout) {
		return err
	}
	return fmt.Errorf("timed out waiting for fleet %s to be deleted; it may still exist — rerun destroy to retry: %w", fleetID, err)
}

// deleteBuildResource deletes the GameLift build, logging a warning on failure.
func (d *Deployer) deleteBuildResource(ctx context.Context, buildID string) {
	if buildID == "" {
		return
	}

	fmt.Fprintln(d.output(), "Deleting build...")
	_, err := d.glClient.DeleteBuild(ctx, &gamelift.DeleteBuildInput{
		BuildId: aws.String(buildID),
	})
	if err != nil && !awsutil.IsNotFound(err) {
		fmt.Fprintf(d.output(), "Warning: failed to delete build: %v\n", err)
		return
	}
	fmt.Fprintln(d.output(), "Build deleted.")
}
