package ec2fleet

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"

	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/state"
)

// newOutputTestDeployer builds a Deployer whose fleet, S3, and IAM clients
// are fakes, so destroy prints can be driven without AWS.
// DescribeFleetAttributes returns no fleets, which the deletion poll treats
// as gone.
func newOutputTestDeployer() *Deployer {
	return &Deployer{
		opts:      DeployOptions{FleetName: "test-fleet"},
		glClient:  &fakeGLClient{describeOut: &gamelift.DescribeFleetAttributesOutput{}},
		s3Client:  &fakeS3Client{},
		iamClient: &fakeIAMClient{getRoleErr: notFoundPolicyErr()},
	}
}

// TestSetOutputRoutesDestroyTranscript pins #765: after SetOutput, the
// deployer's teardown prints land in the routed writer instead of the
// process stdout.
func TestSetOutputRoutesDestroyTranscript(t *testing.T) {
	d := newOutputTestDeployer()

	var buf bytes.Buffer
	d.SetOutput(&buf)

	if err := d.Destroy(context.Background(), "fleet-1", "build-1", "bucket-1", "key-1"); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}

	for _, want := range []string{
		"Deleting fleet...",
		"Fleet deleted.",
		"Deleting build...",
		"Deleting S3 object s3://bucket-1/key-1...",
		"Deleting IAM role...",
		"IAM role not found, skipping.",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("routed output missing %q", want)
		}
	}
}

// TestAdapterSetOutputRoutesNoStateNote drives the adapter destroy path with
// no recorded state, so the not-deployed note lands in the routed writer.
func TestAdapterSetOutputRoutesNoStateNote(t *testing.T) {
	t.Chdir(t.TempDir())
	a := NewTargetAdapter(newOutputTestDeployer())

	var buf bytes.Buffer
	a.SetOutput(&buf)

	if err := a.Destroy(context.Background()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if !strings.Contains(buf.String(), "No EC2 fleet deployment state found.") {
		t.Errorf("routed output = %q, want the no-state note", buf.String())
	}
}

// TestSetOutputRepointsRunner pins the runner half of #765: the runner is
// constructed against the global streams before the deployer is wired, so
// SetOutput must re-derive its subprocess output from the capture.
func TestSetOutputRepointsRunner(t *testing.T) {
	r := runner.NewRunner(false, false)
	if r.Stdout == nil || r.Stderr == nil {
		t.Fatal("precondition: runner must start with default streams")
	}
	d := NewDeployer(DeployOptions{}, aws.Config{}, r)

	var buf bytes.Buffer
	d.SetOutput(&buf)

	if d.Runner.Stdout != io.Writer(&buf) || d.Runner.Stderr != io.Writer(&buf) {
		t.Error("SetOutput did not repoint the runner's subprocess streams")
	}
}

// TestAdapterImplementsOutputSetter pins the wiring contract: the EC2
// adapter satisfies deploy.OutputSetter so MCP handlers route their
// per-operation capture (#765).
func TestAdapterImplementsOutputSetter(t *testing.T) {
	var target deploy.Target = NewTargetAdapter(newOutputTestDeployer())
	if _, ok := target.(deploy.OutputSetter); !ok {
		t.Error("TargetAdapter does not implement deploy.OutputSetter")
	}
}

// TestAdapterDestroyClearsState drives the post-teardown state clear with a
// recorded deployment, so the clear warnings resolve through the routed
// writer and the state blocks are actually removed.
func TestAdapterDestroyClearsState(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := state.UpdateEC2Fleet(&state.EC2FleetState{FleetID: "fleet-1", BuildID: "build-1"}); err != nil {
		t.Fatalf("UpdateEC2Fleet: %v", err)
	}
	a := NewTargetAdapter(newOutputTestDeployer())

	var buf bytes.Buffer
	a.SetOutput(&buf)

	if err := a.Destroy(context.Background()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Deleting fleet...") {
		t.Errorf("routed output = %q, want the teardown transcript", buf.String())
	}
	st, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if st.EC2Fleet != nil {
		t.Errorf("EC2Fleet state = %+v, want cleared", st.EC2Fleet)
	}
}
