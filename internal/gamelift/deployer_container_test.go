package gamelift

import (
	"context"
	"errors"
	"strings"
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
	for _, tt := range append(definitionMatchCases(), portMatchCases()...) {
		t.Run(tt.name, func(t *testing.T) {
			if got := definitionMatches(tt.current, tt.desired); got != tt.want {
				t.Errorf("definitionMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// definitionMatchCase is one definitionMatches() probe.
type definitionMatchCase struct {
	name    string
	current *gltypes.ContainerGroupDefinition
	desired *gamelift.CreateContainerGroupDefinitionInput
	want    bool
}

// matchProbe builds a case from a mutation applied to a matching definition.
// A nil mutate keeps the definition unchanged.
func matchProbe(name string, mutate func(*gltypes.ContainerGroupDefinition), desired *gamelift.CreateContainerGroupDefinitionInput, want bool) definitionMatchCase {
	return definitionMatchCase{name: name, current: matchingDefinition(mutate), desired: desired, want: want}
}

// definitionMatchCases are the definition-level mismatch probes.
func definitionMatchCases() []definitionMatchCase {
	return []definitionMatchCase{
		matchProbe("exact match", nil, cgdDesiredInput(), true),
		{name: "nil current", desired: cgdDesiredInput()},
		matchProbe("nil desired", nil, nil, false),
		matchProbe("current has no game server container", func(d *gltypes.ContainerGroupDefinition) {
			d.GameServerContainerDefinition = nil
		}, cgdDesiredInput(), false),
		matchProbe("image mismatch", func(d *gltypes.ContainerGroupDefinition) {
			d.GameServerContainerDefinition.ImageUri = aws.String("other:latest")
		}, cgdDesiredInput(), false),
		matchProbe("sdk version mismatch", func(d *gltypes.ContainerGroupDefinition) {
			d.GameServerContainerDefinition.ServerSdkVersion = aws.String("5.5.0")
		}, cgdDesiredInput(), false),
		matchProbe("memory limit mismatch", func(d *gltypes.ContainerGroupDefinition) {
			d.TotalMemoryLimitMebibytes = aws.Int32(2048)
		}, cgdDesiredInput(), false),
		matchProbe("vcpu limit mismatch", func(d *gltypes.ContainerGroupDefinition) {
			d.TotalVcpuLimit = aws.Float64(2.0)
		}, cgdDesiredInput(), false),
	}
}

// portMatchCases are the port-configuration mismatch probes.
func portMatchCases() []definitionMatchCase {
	const (
		udp = gltypes.IpProtocolUdp
		tcp = gltypes.IpProtocolTcp
	)
	return []definitionMatchCase{
		matchProbe("current port configuration missing", dropPortConfig, cgdDesiredInput(), false),
		matchProbe("desired port configuration missing", nil, withoutPortConfiguration(cgdDesiredInput()), false),
		matchProbe("both port configurations missing", dropPortConfig, withoutPortConfiguration(cgdDesiredInput()), true),
		matchProbe("port range count mismatch", func(d *gltypes.ContainerGroupDefinition) {
			d.GameServerContainerDefinition.PortConfiguration.ContainerPortRanges = append(
				d.GameServerContainerDefinition.PortConfiguration.ContainerPortRanges,
				gltypes.ContainerPortRange{FromPort: aws.Int32(1), ToPort: aws.Int32(2), Protocol: udp},
			)
		}, cgdDesiredInput(), false),
		matchProbe("from port mismatch", func(d *gltypes.ContainerGroupDefinition) {
			d.GameServerContainerDefinition.PortConfiguration.ContainerPortRanges[0].FromPort = aws.Int32(10)
		}, cgdDesiredInput(), false),
		matchProbe("to port mismatch", func(d *gltypes.ContainerGroupDefinition) {
			d.GameServerContainerDefinition.PortConfiguration.ContainerPortRanges[0].ToPort = aws.Int32(10)
		}, cgdDesiredInput(), false),
		matchProbe("protocol mismatch", func(d *gltypes.ContainerGroupDefinition) {
			d.GameServerContainerDefinition.PortConfiguration.ContainerPortRanges[0].Protocol = tcp
		}, cgdDesiredInput(), false),
	}
}

// dropPortConfig removes the current definition's port configuration while
// keeping the rest matching.
func dropPortConfig(d *gltypes.ContainerGroupDefinition) {
	d.GameServerContainerDefinition.PortConfiguration = nil
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

// cgdDeleteCase is one deleteContainerGroupDefinition() probe.
type cgdDeleteCase struct {
	name             string
	fleet            *fakeFleetClient
	destroyedFleetID string
	deleteErr        error
	wantErrSub       string
	wantDeletes      int
	wantNoteSub      string
}

// cgdListingFleets builds a fleet client whose single listing page returns one
// ACTIVE fleet per ID; tagsByARN resolves each fleet's ownership tag.
func cgdListingFleets(tagsByARN map[string]*gamelift.ListTagsForResourceOutput, fleetIDs ...string) *fakeFleetClient {
	page := &gamelift.ListContainerFleetsOutput{}
	for _, id := range fleetIDs {
		page.ContainerFleets = append(page.ContainerFleets, fleetPage(id, "ACTIVE", "").ContainerFleets[0])
	}
	return &fakeFleetClient{listOut: page, tagsByARN: tagsByARN}
}

// cgdTagByARN maps one fleet's ARN to a ludus fleet-name ownership tag.
func cgdTagByARN(fleetID, fleetName string) map[string]*gamelift.ListTagsForResourceOutput {
	return map[string]*gamelift.ListTagsForResourceOutput{
		"arn:aws:gamelift:us-east-1::fleet/" + fleetID: fleetTagsOut(fleetName),
	}
}

// newTestCGDTeardownDeployer wires a deployer for container-group-definition
// teardown tests: the fleet client drives the shared-reference listing while
// the CGD client records delete calls.
func newTestCGDTeardownDeployer(fleet *fakeFleetClient, cgd *fakeCGDClient) *Deployer {
	return &Deployer{
		opts:      DeployOptions{ContainerGroupName: "ludus-test"},
		glClient:  fleet,
		cgdClient: cgd,
	}
}

// cgdDeleteLifecycleCases are the raw delete-call probes: an unreferenced
// definition is deleted, and delete errors keep their pre-#768 handling. The
// stale-listing case pins that the destroyed fleet itself is excluded from
// the reference check.
func cgdDeleteLifecycleCases() []cgdDeleteCase {
	return []cgdDeleteCase{
		{name: "success", fleet: cgdListingFleets(nil), wantDeletes: 1},
		{
			name:        "already deleted treated as success",
			fleet:       cgdListingFleets(nil),
			deleteErr:   &cgdAPIError{code: "NotFoundException"},
			wantDeletes: 1,
		},
		{
			name:        "delete error propagates",
			fleet:       cgdListingFleets(nil),
			deleteErr:   &cgdAPIError{code: "AccessDeniedException"},
			wantErrSub:  "deleting container group definition",
			wantDeletes: 1,
		},
		{
			name:             "stale listing of the destroyed fleet still deletes",
			fleet:            cgdListingFleets(nil, "fleet-1"),
			destroyedFleetID: "fleet-1",
			wantDeletes:      1,
		},
	}
}

// cgdDeleteReferenceCases are the #768 shared-definition probes: any other
// fleet referencing the definition keeps it alive, and so does a listing the
// reference check could not complete.
func cgdDeleteReferenceCases() []cgdDeleteCase {
	return []cgdDeleteCase{
		{
			name:             "another deployment's fleet keeps the definition",
			fleet:            cgdListingFleets(cgdTagByARN("fleet-2", "blue"), "fleet-2"),
			destroyedFleetID: "fleet-1",
			wantNoteSub:      "other fleet(s): fleet-2 (deployment: blue)",
		},
		{
			name:             "untagged fleet keeps the definition",
			fleet:            cgdListingFleets(nil, "fleet-2"),
			destroyedFleetID: "fleet-1",
			wantNoteSub:      "other fleet(s): fleet-2 (untagged)",
		},
		{
			name:             "stale listing still skips when another deployment shares the definition",
			fleet:            cgdListingFleets(cgdTagByARN("fleet-2", "blue"), "fleet-1", "fleet-2"),
			destroyedFleetID: "fleet-1",
			wantNoteSub:      "other fleet(s): fleet-2 (deployment: blue)",
		},
		{
			name:        "listing error skips deletion",
			fleet:       &fakeFleetClient{listErr: &cgdAPIError{code: "AccessDeniedException"}},
			wantNoteSub: "Skipping container group definition deletion",
		},
	}
}

func TestDeleteContainerGroupDefinition(t *testing.T) {
	for _, tt := range append(cgdDeleteLifecycleCases(), cgdDeleteReferenceCases()...) {
		t.Run(tt.name, func(t *testing.T) {
			cgd := &fakeCGDClient{deleteResults: []cgdDeleteResult{{err: tt.deleteErr}}}
			var err error
			out := captureStdout(func() {
				err = newTestCGDTeardownDeployer(tt.fleet, cgd).
					deleteContainerGroupDefinition(context.Background(), tt.destroyedFleetID)
			})
			if tt.wantErrSub != "" {
				assertErrorContains(t, err, tt.wantErrSub)
			} else if err != nil {
				t.Fatalf("deleteContainerGroupDefinition() error = %v", err)
			}
			if tt.wantNoteSub != "" && !strings.Contains(out, tt.wantNoteSub) {
				t.Errorf("deleteContainerGroupDefinition() output = %q, want it to contain %q", out, tt.wantNoteSub)
			}
			if cgd.deleteCalls != tt.wantDeletes {
				t.Errorf("delete calls = %d, want %d", cgd.deleteCalls, tt.wantDeletes)
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
