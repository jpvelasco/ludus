package anywhere

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/jpvelasco/ludus/internal/runner"
)

func TestTargetAdapter(t *testing.T) {
	d := NewDeployer(DeployOptions{FleetName: "f"}, aws.Config{}, runner.NewRunner(false, true))
	a := NewTargetAdapter(d)

	if a.Name() != "anywhere" {
		t.Errorf("Name() = %q, want anywhere", a.Name())
	}
	if a.Deployer() != d {
		t.Error("Deployer() did not return the wrapped deployer")
	}

	caps := a.Capabilities()
	if caps.NeedsContainerBuild || caps.NeedsContainerPush {
		t.Error("anywhere should not need container build/push")
	}
	if !caps.SupportsSession || !caps.SupportsDeploy || !caps.SupportsDestroy {
		t.Errorf("anywhere should support session/deploy/destroy, got %+v", caps)
	}
}

func TestRollbackLaunchFailure(t *testing.T) {
	// A failed launch must tear down the fleet and compute, but must NOT delete a
	// location that was reused (not created by this attempt). Drive both cases
	// against a fake GameLift client and assert exactly which API calls happen.
	tests := []struct {
		name            string
		locationName    string
		locationCreated bool
		wantDeleteLoc   bool
	}{
		{"created location is deleted", "custom-loc", true, true},
		{"reused location is preserved", "custom-loc", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeGameLift{}
			d := NewDeployer(DeployOptions{FleetName: "f", LocationName: tt.locationName},
				aws.Config{}, runner.NewRunner(false, true))
			d.glClient = fake
			a := NewTargetAdapter(d)

			a.rollbackLaunchFailure(context.Background(), "fleet-123", "compute-abc", tt.locationCreated)

			if !fake.deletedFleet {
				t.Error("rollback must delete the fleet")
			}
			if !fake.deregisteredCompute {
				t.Error("rollback must deregister the compute")
			}
			if fake.deletedLocation != tt.wantDeleteLoc {
				t.Errorf("deletedLocation = %v, want %v", fake.deletedLocation, tt.wantDeleteLoc)
			}
		})
	}
}
