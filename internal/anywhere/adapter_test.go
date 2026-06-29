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
	// With empty fleet/compute IDs and an empty location name, every Destroy
	// sub-step early-returns (no AWS calls), so this exercises the rollback
	// control flow without touching GameLift. Both branches of the
	// locationCreated guard are covered:
	//   - false: the reused location is preserved (empty name → not deleted)
	//   - true:  the created location name is passed through (still empty here,
	//            so Destroy's deleteLocation guard early-returns)
	d := NewDeployer(DeployOptions{FleetName: "f"}, aws.Config{}, runner.NewRunner(false, true))
	a := NewTargetAdapter(d)

	for _, created := range []bool{false, true} {
		a.rollbackLaunchFailure(context.Background(), "", "", created)
	}
}
