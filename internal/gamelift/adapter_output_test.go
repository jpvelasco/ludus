package gamelift

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/gamelift"

	"github.com/jpvelasco/ludus/internal/deploy"
)

// TestAdapterSetOutputRoutesDeployTranscript pins #765: SetOutput routes the
// adapter's own status prints and the deployer's prints into the given writer
// (an MCP tool's per-operation capture) instead of the process stdout.
func TestAdapterSetOutputRoutesDeployTranscript(t *testing.T) {
	a := newAdapter(t)

	var buf bytes.Buffer
	a.SetOutput(&buf)

	if _, err := a.Deploy(context.Background(), deploy.DeployInput{}); err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}

	for _, want := range []string{
		"Creating container group definition...",
		"Container group definition ready: arn:cgd-adapter",
		"Creating fleet...",
		"Fleet status: ACTIVE",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("routed output missing %q", want)
		}
	}
}

// TestAdapterSetOutputRoutesDestroyTranscript drives the destroy path through
// the routed writer so teardown notes (including the shared-definition skip
// note from #770) land in the capture.
func TestAdapterSetOutputRoutesDestroyTranscript(t *testing.T) {
	a := newAdapter(t)
	fleet := a.deployer.glClient.(*fakeFleetClient)
	fleet.listOut = &gamelift.ListContainerFleetsOutput{}

	var buf bytes.Buffer
	a.SetOutput(&buf)

	if err := a.Destroy(context.Background()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}

	for _, want := range []string{
		"Deleting fleet...",
		"Deleting container group definition...",
		"Deleting IAM role...",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("routed output missing %q", want)
		}
	}
}

// TestAdapterSetOutputDelegatesToDeployer pins the delegation chain: the
// adapter's SetOutput must store the writer on the wrapped deployer so the
// deployer's direct callers (CLI fleet commands) share one routing owner.
func TestAdapterSetOutputDelegatesToDeployer(t *testing.T) {
	d := &Deployer{}
	a := NewTargetAdapter(d)

	var buf bytes.Buffer
	a.SetOutput(&buf)

	if d.outputWriter != &buf {
		t.Error("SetOutput did not store the writer on the wrapped deployer")
	}
}
