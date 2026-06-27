// Package gamelift implements the deploy.Target adapter and supporting
// logic for AWS GameLift container fleets (including container group
// definitions, fleet creation, and IAM roles).
package gamelift

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/tags"
)

func (d *Deployer) containerGroupDefinitionInput() *gamelift.CreateContainerGroupDefinitionInput {
	sdkVersion := d.opts.ServerSDKVersion
	if sdkVersion == "" {
		sdkVersion = "5.4.0"
	}

	return &gamelift.CreateContainerGroupDefinitionInput{
		Name:                      aws.String(d.opts.ContainerGroupName),
		OperatingSystem:           gltypes.ContainerOperatingSystemAmazonLinux2023,
		TotalMemoryLimitMebibytes: aws.Int32(1024),
		TotalVcpuLimit:            aws.Float64(1.0),
		Tags:                      tags.ToGameLiftTags(d.resourceTags()),
		GameServerContainerDefinition: &gltypes.GameServerContainerDefinitionInput{
			ContainerName:    aws.String("game-server"),
			ImageUri:         aws.String(d.opts.ImageURI),
			ServerSdkVersion: aws.String(sdkVersion),
			PortConfiguration: &gltypes.ContainerPortConfiguration{
				ContainerPortRanges: []gltypes.ContainerPortRange{
					{
						FromPort: aws.Int32(int32(d.opts.ServerPort)),
						ToPort:   aws.Int32(int32(d.opts.ServerPort)),
						Protocol: gltypes.IpProtocolUdp,
					},
				},
			},
		},
	}
}

// definitionMatches returns whether the on-disk (described) container group definition
// is equivalent to what we want to create right now. Used to safely decide reuse vs replace.
func definitionMatches(current *gltypes.ContainerGroupDefinition, desired *gamelift.CreateContainerGroupDefinitionInput) bool {
	if current == nil || desired == nil || current.GameServerContainerDefinition == nil || desired.GameServerContainerDefinition == nil {
		return false
	}
	c := current.GameServerContainerDefinition
	d := desired.GameServerContainerDefinition
	if aws.ToString(c.ImageUri) != aws.ToString(d.ImageUri) {
		return false
	}
	if aws.ToString(c.ServerSdkVersion) != aws.ToString(d.ServerSdkVersion) {
		return false
	}
	// Normalize nils for numeric fields (desired always sets them; described may omit)
	if aws.ToInt32(current.TotalMemoryLimitMebibytes) != aws.ToInt32(desired.TotalMemoryLimitMebibytes) {
		return false
	}
	if aws.ToFloat64(current.TotalVcpuLimit) != aws.ToFloat64(desired.TotalVcpuLimit) {
		return false
	}
	// Compare port configuration (desired sets from ServerPort)
	cPorts := c.PortConfiguration
	dPorts := d.PortConfiguration
	if (cPorts == nil) != (dPorts == nil) {
		return false
	}
	if cPorts != nil && dPorts != nil {
		if len(cPorts.ContainerPortRanges) != len(dPorts.ContainerPortRanges) {
			return false
		}
		for i := range cPorts.ContainerPortRanges {
			cr, dr := cPorts.ContainerPortRanges[i], dPorts.ContainerPortRanges[i]
			if aws.ToInt32(cr.FromPort) != aws.ToInt32(dr.FromPort) ||
				aws.ToInt32(cr.ToPort) != aws.ToInt32(dr.ToPort) ||
				cr.Protocol != dr.Protocol {
				return false
			}
		}
	}
	return true
}

func (d *Deployer) waitForContainerGroupReady(ctx context.Context) error {
	err := awsutil.Poll(ctx, pollInterval, maxPollWait, func() (bool, error) {
		desc, err := d.glClient.DescribeContainerGroupDefinition(ctx, &gamelift.DescribeContainerGroupDefinitionInput{
			Name: aws.String(d.opts.ContainerGroupName),
		})
		if err != nil {
			return false, fmt.Errorf("polling container group definition status: %w", err)
		}

		status := desc.ContainerGroupDefinition.Status
		fmt.Printf("  Container group definition status: %s\n", status)
		if status == gltypes.ContainerGroupDefinitionStatusReady {
			return true, nil
		}
		if status == gltypes.ContainerGroupDefinitionStatusFailed {
			reason := aws.ToString(desc.ContainerGroupDefinition.StatusReason)
			return false, fmt.Errorf("container group definition failed: %s", reason)
		}
		return false, nil
	})
	return awsutil.WrapTimeout(err, "container group definition to become READY")
}

func (d *Deployer) deleteContainerGroupDefinition(ctx context.Context) error {
	fmt.Println("Deleting container group definition...")

	_, err := d.glClient.DeleteContainerGroupDefinition(ctx, &gamelift.DeleteContainerGroupDefinitionInput{
		Name: aws.String(d.opts.ContainerGroupName),
	})
	if err != nil {
		if awsutil.IsNotFound(err) {
			fmt.Println("Container group definition not found, skipping.")
			return nil
		}
		return fmt.Errorf("deleting container group definition: %w", err)
	}

	fmt.Println("Container group definition deleted.")
	return nil
}
