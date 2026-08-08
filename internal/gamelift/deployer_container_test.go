package gamelift

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

// cgdDesiredInput mirrors the input produced by a deployer with empty
// ImageURI/ServerPort, matching the matchingCGDOutput fixture.
func cgdDesiredInput() *gamelift.CreateContainerGroupDefinitionInput {
	return (&Deployer{opts: DeployOptions{}}).containerGroupDefinitionInput()
}

// matchingDefinition returns a described definition that matches
// cgdDesiredInput() exactly, with an optional mutation hook per case.
func matchingDefinition(mutate func(*gltypes.ContainerGroupDefinition)) *gltypes.ContainerGroupDefinition {
	def := matchingCGDOutput("arn:match").ContainerGroupDefinition
	if mutate != nil {
		mutate(def)
	}
	return def
}

func TestDefinitionMatches(t *testing.T) {
	const (
		udp = gltypes.IpProtocolUdp
		tcp = gltypes.IpProtocolTcp
	)

	tests := []struct {
		name    string
		current *gltypes.ContainerGroupDefinition
		desired *gamelift.CreateContainerGroupDefinitionInput
		want    bool
	}{
		{
			name:    "exact match",
			current: matchingDefinition(nil),
			desired: cgdDesiredInput(),
			want:    true,
		},
		{
			name:    "nil current",
			current: nil,
			desired: cgdDesiredInput(),
		},
		{
			name:    "nil desired",
			current: matchingDefinition(nil),
			desired: nil,
		},
		{
			name: "current has no game server container",
			current: matchingDefinition(func(d *gltypes.ContainerGroupDefinition) {
				d.GameServerContainerDefinition = nil
			}),
			desired: cgdDesiredInput(),
		},
		{
			name: "image mismatch",
			current: matchingDefinition(func(d *gltypes.ContainerGroupDefinition) {
				d.GameServerContainerDefinition.ImageUri = aws.String("other:latest")
			}),
			desired: cgdDesiredInput(),
		},
		{
			name: "sdk version mismatch",
			current: matchingDefinition(func(d *gltypes.ContainerGroupDefinition) {
				d.GameServerContainerDefinition.ServerSdkVersion = aws.String("5.5.0")
			}),
			desired: cgdDesiredInput(),
		},
		{
			name: "memory limit mismatch",
			current: matchingDefinition(func(d *gltypes.ContainerGroupDefinition) {
				d.TotalMemoryLimitMebibytes = aws.Int32(2048)
			}),
			desired: cgdDesiredInput(),
		},
		{
			name: "vcpu limit mismatch",
			current: matchingDefinition(func(d *gltypes.ContainerGroupDefinition) {
				d.TotalVcpuLimit = aws.Float64(2.0)
			}),
			desired: cgdDesiredInput(),
		},
		{
			name: "current port configuration missing",
			current: matchingDefinition(func(d *gltypes.ContainerGroupDefinition) {
				d.GameServerContainerDefinition.PortConfiguration = nil
			}),
			desired: cgdDesiredInput(),
		},
		{
			name:    "desired port configuration missing",
			current: matchingDefinition(nil),
			desired: withoutPortConfiguration(cgdDesiredInput()),
		},
		{
			name: "both port configurations missing",
			current: matchingDefinition(func(d *gltypes.ContainerGroupDefinition) {
				d.GameServerContainerDefinition.PortConfiguration = nil
			}),
			desired: withoutPortConfiguration(cgdDesiredInput()),
			want:    true,
		},
		{
			name: "port range count mismatch",
			current: matchingDefinition(func(d *gltypes.ContainerGroupDefinition) {
				d.GameServerContainerDefinition.PortConfiguration.ContainerPortRanges = append(
					d.GameServerContainerDefinition.PortConfiguration.ContainerPortRanges,
					gltypes.ContainerPortRange{FromPort: aws.Int32(1), ToPort: aws.Int32(2), Protocol: udp},
				)
			}),
			desired: cgdDesiredInput(),
		},
		{
			name: "from port mismatch",
			current: matchingDefinition(func(d *gltypes.ContainerGroupDefinition) {
				d.GameServerContainerDefinition.PortConfiguration.ContainerPortRanges[0].FromPort = aws.Int32(10)
			}),
			desired: cgdDesiredInput(),
		},
		{
			name: "to port mismatch",
			current: matchingDefinition(func(d *gltypes.ContainerGroupDefinition) {
				d.GameServerContainerDefinition.PortConfiguration.ContainerPortRanges[0].ToPort = aws.Int32(10)
			}),
			desired: cgdDesiredInput(),
		},
		{
			name: "protocol mismatch",
			current: matchingDefinition(func(d *gltypes.ContainerGroupDefinition) {
				d.GameServerContainerDefinition.PortConfiguration.ContainerPortRanges[0].Protocol = tcp
			}),
			desired: cgdDesiredInput(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := definitionMatches(tt.current, tt.desired); got != tt.want {
				t.Errorf("definitionMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func withoutPortConfiguration(in *gamelift.CreateContainerGroupDefinitionInput) *gamelift.CreateContainerGroupDefinitionInput {
	in.GameServerContainerDefinition.PortConfiguration = nil
	return in
}

func TestWaitForContainerGroupReady(t *testing.T) {
	t.Run("returns when ready", func(t *testing.T) {
		client := &fakeCGDClient{
			describeResults: []cgdDescribeResult{{out: readyCGDOutput("arn:cgd")}},
		}
		if err := newTestCGDDeployer(client).waitForContainerGroupReady(context.Background()); err != nil {
			t.Fatalf("waitForContainerGroupReady() error = %v", err)
		}
	})

	t.Run("fails with status reason", func(t *testing.T) {
		client := &fakeCGDClient{
			describeResults: []cgdDescribeResult{{
				out: &gamelift.DescribeContainerGroupDefinitionOutput{
					ContainerGroupDefinition: &gltypes.ContainerGroupDefinition{
						Status:       gltypes.ContainerGroupDefinitionStatusFailed,
						StatusReason: aws.String("image pull failed"),
					},
				},
			}},
		}
		err := newTestCGDDeployer(client).waitForContainerGroupReady(context.Background())
		assertErrorContains(t, err, "container group definition failed: image pull failed")
	})

	t.Run("wraps describe error", func(t *testing.T) {
		client := &fakeCGDClient{
			describeResults: []cgdDescribeResult{{err: &cgdAPIError{code: "InternalServiceException"}}},
		}
		err := newTestCGDDeployer(client).waitForContainerGroupReady(context.Background())
		assertErrorContains(t, err, "polling container group definition status")
	})

	t.Run("keeps polling while in progress", func(t *testing.T) {
		client := &fakeCGDClient{
			describeResults: []cgdDescribeResult{{
				out: &gamelift.DescribeContainerGroupDefinitionOutput{
					ContainerGroupDefinition: &gltypes.ContainerGroupDefinition{
						Status: gltypes.ContainerGroupDefinitionStatusCopying,
					},
				},
			}},
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := newTestCGDDeployer(client).waitForContainerGroupReady(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("waitForContainerGroupReady() error = %v, want context.Canceled", err)
		}
	})
}

func TestDeleteContainerGroupDefinition(t *testing.T) {
	tests := []struct {
		name        string
		deleteErr   error
		wantErrSub  string
		wantDeletes int
	}{
		{name: "success", wantDeletes: 1},
		{name: "already deleted treated as success", deleteErr: &cgdAPIError{code: "NotFoundException"}, wantDeletes: 1},
		{name: "delete error propagates", deleteErr: &cgdAPIError{code: "AccessDeniedException"}, wantErrSub: "deleting container group definition", wantDeletes: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeCGDClient{
				deleteResults: []cgdDeleteResult{{err: tt.deleteErr}},
			}
			err := newTestCGDDeployer(client).deleteContainerGroupDefinition(context.Background())
			if tt.wantErrSub != "" {
				assertErrorContains(t, err, tt.wantErrSub)
			} else if err != nil {
				t.Fatalf("deleteContainerGroupDefinition() error = %v", err)
			}
			if client.deleteCalls != tt.wantDeletes {
				t.Errorf("delete calls = %d, want %d", client.deleteCalls, tt.wantDeletes)
			}
		})
	}
}

func TestWaitForContainerFleetActiveContinuesPolling(t *testing.T) {
	client := &fakeFleetClient{describeOut: &gamelift.DescribeContainerFleetOutput{
		ContainerFleet: &gltypes.ContainerFleet{Status: gltypes.ContainerFleetStatusUpdating},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := &FleetStatus{}
	err := newTestFleetDeployer(client, &fakeIAMClient{}).waitForContainerFleetActive(ctx, "fleet-1", result)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("waitForContainerFleetActive() error = %v, want context.Canceled", err)
	}
	if result.Status != "UPDATING" {
		t.Errorf("result.Status = %q, want UPDATING recorded before cancellation", result.Status)
	}
}

func TestWaitForContainerFleetDeletionContinuesPolling(t *testing.T) {
	client := &fakeFleetClient{describeOut: &gamelift.DescribeContainerFleetOutput{
		ContainerFleet: &gltypes.ContainerFleet{Status: gltypes.ContainerFleetStatusDeleting},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := newTestFleetDeployer(client, &fakeIAMClient{}).waitForContainerFleetDeletion(ctx, "fleet-1")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("waitForContainerFleetDeletion() error = %v, want context.Canceled", err)
	}
}
