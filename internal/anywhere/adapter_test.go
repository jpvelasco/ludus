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
	// With empty fleet/compute IDs and a non-created location, every Destroy
	// sub-step early-returns (no AWS calls), so this exercises the rollback
	// control flow — including the locationCreated=false guard that must NOT
	// pass a location name to Destroy — without touching GameLift.
	d := NewDeployer(DeployOptions{FleetName: "f", LocationName: "loc"}, aws.Config{}, runner.NewRunner(false, true))
	a := NewTargetAdapter(d)

	// Should not panic and should complete; locationCreated=false means the
	// reused location is preserved (not deleted).
	a.rollbackLaunchFailure(context.Background(), "", "", false)
}
