package stack

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/jpvelasco/ludus/internal/deploy"
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
	want := deploy.Capabilities{
		NeedsContainerBuild: true,
		NeedsContainerPush:  true,
		NeedsAWS:            true,
		NeedsImageURI:       true,
		SupportsSession:     true,
		SupportsDeploy:      true,
		SupportsDestroy:     true,
	}
	if caps != want {
		t.Errorf("Capabilities() = %+v, want %+v", caps, want)
	}
}
