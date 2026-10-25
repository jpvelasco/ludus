package anywhere

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/runner"
)

// TestSetOutputRoutesDestroyTranscript pins #765: after SetOutput, the
// deployer's teardown prints land in the routed writer instead of the
// process stdout. The fleet and compute identifiers are empty, so the
// location teardown drives the printable surface.
func TestSetOutputRoutesDestroyTranscript(t *testing.T) {
	fake := &fakeGameLift{}
	d := NewDeployer(DeployOptions{LocationName: "custom-loc"}, aws.Config{}, runner.NewRunner(false, true))
	d.glClient = fake

	var buf bytes.Buffer
	d.SetOutput(&buf)

	if err := d.Destroy(context.Background(), "fleet-1", "compute-1", "custom-loc", 0, ""); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}

	for _, want := range []string{
		"Deregistering compute...",
		"Deleting fleet...",
		"Deleting location...",
		"Location deleted.",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("routed output missing %q", want)
		}
	}
}

// TestAdapterSetOutputRoutesRollbackNote drives the adapter's rollback print
// through the routed writer — the print surface reachable without a packaged
// server build.
func TestAdapterSetOutputRoutesRollbackNote(t *testing.T) {
	fake := &fakeGameLift{}
	d := NewDeployer(DeployOptions{FleetName: "f", LocationName: "custom-loc"}, aws.Config{}, runner.NewRunner(false, true))
	d.glClient = fake
	a := NewTargetAdapter(d)

	var buf bytes.Buffer
	a.SetOutput(&buf)

	a.rollbackPartialResources(context.Background(), "fleet-123", "compute-abc", false, "")

	if !strings.Contains(buf.String(), "Deployment failed; rolling back Anywhere resources...") {
		t.Errorf("routed output = %q, want the rollback note", buf.String())
	}
}

// TestAdapterSetOutputRoutesNoStateNote drives the adapter destroy path with
// no recorded state, so the not-deployed note lands in the routed writer.
func TestAdapterSetOutputRoutesNoStateNote(t *testing.T) {
	t.Chdir(t.TempDir())
	d := NewDeployer(DeployOptions{}, aws.Config{}, runner.NewRunner(false, true))
	a := NewTargetAdapter(d)

	var buf bytes.Buffer
	a.SetOutput(&buf)

	if err := a.Destroy(context.Background()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if !strings.Contains(buf.String(), "No Anywhere deployment state found.") {
		t.Errorf("routed output = %q, want the no-state note", buf.String())
	}
}

// TestSetOutputRepointsRunner pins the runner half of #765: the runner is
// constructed against the global streams before the deployer is wired, so
// SetOutput must re-derive its subprocess output from the capture.
func TestSetOutputRepointsRunner(t *testing.T) {
	r := runner.NewRunner(false, false)
	d := NewDeployer(DeployOptions{}, aws.Config{}, r)

	var buf bytes.Buffer
	d.SetOutput(&buf)

	if d.Runner.Stdout != io.Writer(&buf) || d.Runner.Stderr != io.Writer(&buf) {
		t.Error("SetOutput did not repoint the runner's subprocess streams")
	}
}

// TestAdapterSetOutputRoutesCreateResources drives the deploy flow's
// print-producing core through the routed writer: adapter prints, deployer
// prints, and the dry-run launch echo all land in the capture.
func TestAdapterSetOutputRoutesCreateResources(t *testing.T) {
	stubWrapperBinary(t)
	a := newRegressionDeployer(t, &fakeGameLift{}, DeployOptions{
		LocationName:   "ludus-dev",
		IPAddress:      "127.0.0.1",
		ServerBuildDir: stubServerBuild(t, hostExecutableName("LyraServer")),
		ProjectName:    "Lyra",
		ServerTarget:   "LyraServer",
		ServerPort:     7777,
	})

	var buf bytes.Buffer
	a.SetOutput(&buf)

	if _, err := a.createResources(context.Background()); err != nil {
		t.Fatalf("createResources() error = %v", err)
	}

	for _, want := range []string{
		"Ensuring game server wrapper binary...",
		"Creating custom location ludus-dev...",
		"Creating Anywhere fleet ",
		"Fleet created: fleet-test",
		"Registering compute...",
		"Compute registered:",
		"Launching game server...",
		"would launch wrapper",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("routed output missing %q", want)
		}
	}
}

// TestAdapterImplementsOutputSetter pins the wiring contract: the Anywhere
// adapter satisfies deploy.OutputSetter so MCP handlers route their
// per-operation capture (#765).
func TestAdapterImplementsOutputSetter(t *testing.T) {
	var target deploy.Target = NewTargetAdapter(newTestDeployer(&fakeGameLift{}, DeployOptions{}))
	if _, ok := target.(deploy.OutputSetter); !ok {
		t.Error("TargetAdapter does not implement deploy.OutputSetter")
	}
}
