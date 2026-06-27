package stack

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestTargetAdapter(t *testing.T) {
	d := NewStackDeployer(StackOptions{FleetName: "f"}, aws.Config{})
	a := NewTargetAdapter(d)

	if a.Name() != "stack" {
		t.Errorf("Name() = %q, want stack", a.Name())
	}
	if a.Deployer() != d {
		t.Error("Deployer() did not return the wrapped deployer")
	}

	caps := a.Capabilities()
	if !caps.NeedsContainerBuild || !caps.NeedsContainerPush {
		t.Error("stack should need container build/push")
	}
	if !caps.SupportsSession || !caps.SupportsDeploy || !caps.SupportsDestroy {
		t.Errorf("stack should support session/deploy/destroy, got %+v", caps)
	}
}
