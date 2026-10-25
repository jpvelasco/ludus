package stack

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/jpvelasco/ludus/internal/deploy"
)

// TestAdapterSetOutputStoresWriterOnDeployer pins #765's delegation chain: the
// adapter's SetOutput stores the writer on the wrapped deployer, so the
// deployer's status prints resolve to the routed capture.
func TestAdapterSetOutputStoresWriterOnDeployer(t *testing.T) {
	d := NewStackDeployer(StackOptions{FleetName: "f"}, aws.Config{})
	a := NewTargetAdapter(d)

	var buf bytes.Buffer
	a.SetOutput(&buf)

	if d.outputWriter != &buf {
		t.Fatal("SetOutput did not store the writer on the wrapped deployer")
	}
}

// TestSetOutputRoutesStackDeletionNote drives the one stack print reachable
// without AWS (the deletion-poll decision note) through the routed writer.
// The remaining stack prints sit behind concrete CloudFormation calls and
// are E2E-covered.
func TestSetOutputRoutesStackDeletionNote(t *testing.T) {
	d := NewStackDeployer(StackOptions{StackName: "ludus-f"}, aws.Config{})
	a := NewTargetAdapter(d)

	var buf bytes.Buffer
	a.SetOutput(&buf)

	finished, err := d.stackDeletionPollResult("DELETE_COMPLETE", "")
	if err != nil {
		t.Fatalf("stackDeletionPollResult() error = %v", err)
	}
	if !finished {
		t.Fatal("stackDeletionPollResult() finished = false, want true")
	}
	if !strings.Contains(buf.String(), "Stack deleted.") {
		t.Errorf("routed output = %q, want the deletion note", buf.String())
	}
}

// TestAdapterImplementsOutputSetter pins the wiring contract: the stack
// adapter satisfies deploy.OutputSetter so MCP handlers route their
// per-operation capture (#765).
func TestAdapterImplementsOutputSetter(t *testing.T) {
	var target deploy.Target = NewTargetAdapter(NewStackDeployer(StackOptions{}, aws.Config{}))
	if _, ok := target.(deploy.OutputSetter); !ok {
		t.Error("TargetAdapter does not implement deploy.OutputSetter")
	}
}
