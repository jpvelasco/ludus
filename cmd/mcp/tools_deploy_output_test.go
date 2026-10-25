package mcp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
)

// printingOutputTarget is a deploy.Target test double that implements
// deploy.OutputSetter and prints to the routed writer on every operation,
// so handler wiring can assert the per-operation capture end-to-end (#765).
type printingOutputTarget struct {
	name      string
	w         io.Writer
	destroyed bool
}

func (t *printingOutputTarget) Name() string                      { return t.name }
func (t *printingOutputTarget) Capabilities() deploy.Capabilities { return deploy.Capabilities{} }

func (t *printingOutputTarget) SetOutput(w io.Writer) { t.w = w }

func (t *printingOutputTarget) Deploy(ctx context.Context, input deploy.DeployInput) (*deploy.DeployResult, error) {
	t.print("deploying " + t.name)
	return &deploy.DeployResult{TargetName: t.name, Status: "ACTIVE", Detail: "stub deploy"}, nil
}

func (t *printingOutputTarget) Status(ctx context.Context) (*deploy.DeployStatus, error) {
	return &deploy.DeployStatus{TargetName: t.name, Status: "active"}, nil
}

func (t *printingOutputTarget) Destroy(ctx context.Context) error {
	t.destroyed = true
	t.print("destroying " + t.name)
	return nil
}

func (t *printingOutputTarget) print(msg string) {
	if t.w != nil {
		_, _ = io.WriteString(t.w, msg+"\n")
	}
}

// printingSessionTarget adds deploy.SessionManager to the printing stub.
type printingSessionTarget struct {
	printingOutputTarget
}

func (t *printingSessionTarget) CreateSession(ctx context.Context, maxPlayers int) (*deploy.SessionInfo, error) {
	t.print("creating session")
	return &deploy.SessionInfo{SessionID: "sess-1", IPAddress: "127.0.0.1", Port: 7777}, nil
}

func (t *printingSessionTarget) DescribeSession(ctx context.Context, sessionID string) (string, error) {
	return "ACTIVE", nil
}

// decodedResultField unmarshals the tool response's JSON payload and returns
// the given string field, so transcript assertions read the structured result
// rather than raw escaped text.
func decodedResultField(t *testing.T, result *mcpsdk.CallToolResult, field string) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(toolResultText(t, result)), &payload); err != nil {
		t.Fatalf("unmarshaling result payload: %v", err)
	}
	s, _ := payload[field].(string)
	return s
}

// assertOutputContains fails when the response's output field does not carry
// the routed transcript.
func assertOutputContains(t *testing.T, result *mcpsdk.CallToolResult, want string) {
	t.Helper()
	if got := decodedResultField(t, result, "output"); !strings.Contains(got, want) {
		t.Errorf("output = %q, want it to contain %q", got, want)
	}
}

// TestRouteTargetOutput pins the wiring helper: a target implementing
// deploy.OutputSetter receives the capture, and one that doesn't is left on
// its default global streams without a panic.
func TestRouteTargetOutput(t *testing.T) {
	t.Run("setter target receives the capture", func(t *testing.T) {
		target := &printingOutputTarget{name: "gamelift"}
		out := newToolOutput()

		routeTargetOutput(target, out)

		if target.w != io.Writer(out) {
			t.Error("routeTargetOutput did not pass the capture to the setter")
		}
	})

	t.Run("plain target is skipped", func(t *testing.T) {
		target := &testDeployTarget{name: "gamelift"}
		out := newToolOutput()

		routeTargetOutput(target, out)

		if out.String() != "" {
			t.Errorf("capture = %q, want empty for a plain target", out.String())
		}
	})
}

// driveDeployFleet runs the fleet deploy handler against the swapped target.
func driveDeployFleet(t *testing.T) *mcpsdk.CallToolResult {
	t.Helper()
	result, _, err := handleDeployFleet(context.Background(), nil, deployFleetInput{})
	if err != nil {
		t.Fatalf("handleDeployFleet() error = %v", err)
	}
	return result
}

// driveDeployEC2 runs the EC2 deploy handler against the swapped target.
func driveDeployEC2(t *testing.T) *mcpsdk.CallToolResult {
	t.Helper()
	result, _, err := handleDeployEC2(context.Background(), nil, deployEC2Input{})
	if err != nil {
		t.Fatalf("handleDeployEC2() error = %v", err)
	}
	return result
}

// driveDeployAnywhere runs the Anywhere deploy handler against the swapped
// target.
func driveDeployAnywhere(t *testing.T) *mcpsdk.CallToolResult {
	t.Helper()
	result, _, err := handleDeployAnywhere(context.Background(), nil, deployAnywhereInput{})
	if err != nil {
		t.Fatalf("handleDeployAnywhere() error = %v", err)
	}
	return result
}

// TestDeployHandlersCaptureTargetTranscript drives every deploy handler whose
// target resolution is seam-replaceable (globals.ResolveTargetFn) with a
// printing OutputSetter stub, asserting each response carries the routed
// transcript. The stack handler constructs its deployer directly with no
// injection seam and its deployer's first action is a CloudFormation call, so
// its transcript is E2E-covered; its wiring is the same routeTargetOutput
// call pinned by TestRouteTargetOutput.
func TestDeployHandlersCaptureTargetTranscript(t *testing.T) {
	tests := []struct {
		name  string
		drive func(t *testing.T) *mcpsdk.CallToolResult
	}{
		{"deploy fleet", driveDeployFleet},
		{"deploy ec2", driveDeployEC2},
		{"deploy anywhere", driveDeployAnywhere},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			globals.SetGlobals(t, &config.Config{
				AWS:       config.AWSConfig{AccountID: "123456789012", Region: "us-west-2", ECRRepository: "ludus-server"},
				Container: config.ContainerConfig{Tag: "test"},
			})
			globals.SwapResolveTarget(t, func(_ context.Context, _ *config.Config, name string) (deploy.Target, error) {
				return &printingOutputTarget{name: name}, nil
			})

			assertOutputContains(t, tt.drive(t), "deploying")
		})
	}
}

// TestHandleDeploySessionCapturesTargetTranscript pins the session handler
// wiring: the resolved session target's prints land in the response output.
func TestHandleDeploySessionCapturesTargetTranscript(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})
	globals.SwapResolveTarget(t, func(_ context.Context, _ *config.Config, name string) (deploy.Target, error) {
		return &printingSessionTarget{printingOutputTarget{name: name}}, nil
	})

	result, _, err := handleDeploySession(context.Background(), nil, deploySessionInput{MaxPlayers: 4})
	if err != nil {
		t.Fatalf("handleDeploySession() error = %v", err)
	}
	assertOutputContains(t, result, "creating session")
}

// TestHandleDeployDestroyCapturesTargetTranscript pins the destroy wiring:
// the resolved target's routed teardown prints join the handler-owned sweep
// notes in the same per-operation capture.
func TestHandleDeployDestroyCapturesTargetTranscript(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})
	globals.SwapResolveTarget(t, func(_ context.Context, _ *config.Config, name string) (deploy.Target, error) {
		return &printingOutputTarget{name: name}, nil
	})

	result, _, err := handleDeployDestroy(context.Background(), nil, deployDestroyInput{Target: "gamelift"})
	if err != nil {
		t.Fatalf("handleDeployDestroy() error = %v", err)
	}
	assertOutputContains(t, result, "destroying gamelift")
}

// TestDestroyAllTargetsRoutesEachTarget pins the sweep wiring: every resolved
// target receives the shared capture before its teardown runs, so all
// per-target transcripts land in one response.
func TestDestroyAllTargetsRoutesEachTarget(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})
	out := newToolOutput()

	globals.SwapResolveTarget(t, func(_ context.Context, _ *config.Config, name string) (deploy.Target, error) {
		return &printingOutputTarget{name: name}, nil
	})

	err := runDestroyForMCP(context.Background(), &config.Config{}, deployDestroyInput{AllTargets: true}, out)
	if err != nil {
		t.Fatalf("runDestroyForMCP() error = %v", err)
	}
	for _, name := range []string{"gamelift", "stack", "ec2", "anywhere", "binary"} {
		if !strings.Contains(out.String(), "destroying "+name) {
			t.Errorf("sweep transcript missing %q", "destroying "+name)
		}
	}
}
