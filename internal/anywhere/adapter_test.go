package anywhere

import (
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
