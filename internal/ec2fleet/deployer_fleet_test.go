package ec2fleet

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/aws/smithy-go"

	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/config"
)

// fakeGLClient implements fleetClient with canned responses and call
// capture, letting tests drive the real EC2 Deployer.CreateFleet hermetically.
type fakeGLClient struct {
	createFleetOut *gamelift.CreateFleetOutput
	createFleetErr error
	describeOut    *gamelift.DescribeFleetAttributesOutput
	describeErr    error

	createFleetCalls   int
	createFleetInput   *gamelift.CreateFleetInput
	describeFleetCalls int
	deleteFleetCalls   int
	deleteBuildCalls   int
}

func (f *fakeGLClient) CreateFleet(_ context.Context, in *gamelift.CreateFleetInput, _ ...func(*gamelift.Options)) (*gamelift.CreateFleetOutput, error) {
	f.createFleetCalls++
	f.createFleetInput = in
	return f.createFleetOut, f.createFleetErr
}

func (f *fakeGLClient) DescribeFleetAttributes(_ context.Context, _ *gamelift.DescribeFleetAttributesInput, _ ...func(*gamelift.Options)) (*gamelift.DescribeFleetAttributesOutput, error) {
	f.describeFleetCalls++
	return f.describeOut, f.describeErr
}

func (f *fakeGLClient) ListFleets(_ context.Context, _ *gamelift.ListFleetsInput, _ ...func(*gamelift.Options)) (*gamelift.ListFleetsOutput, error) {
	return &gamelift.ListFleetsOutput{}, nil
}

func (f *fakeGLClient) DeleteFleet(_ context.Context, _ *gamelift.DeleteFleetInput, _ ...func(*gamelift.Options)) (*gamelift.DeleteFleetOutput, error) {
	f.deleteFleetCalls++
	return &gamelift.DeleteFleetOutput{}, nil
}

func (f *fakeGLClient) CreateBuild(_ context.Context, _ *gamelift.CreateBuildInput, _ ...func(*gamelift.Options)) (*gamelift.CreateBuildOutput, error) {
	return &gamelift.CreateBuildOutput{}, nil
}

func (f *fakeGLClient) DescribeBuild(_ context.Context, _ *gamelift.DescribeBuildInput, _ ...func(*gamelift.Options)) (*gamelift.DescribeBuildOutput, error) {
	return &gamelift.DescribeBuildOutput{}, nil
}

func (f *fakeGLClient) DeleteBuild(_ context.Context, _ *gamelift.DeleteBuildInput, _ ...func(*gamelift.Options)) (*gamelift.DeleteBuildOutput, error) {
	f.deleteBuildCalls++
	return &gamelift.DeleteBuildOutput{}, nil
}

func (f *fakeGLClient) CreateGameSession(_ context.Context, _ *gamelift.CreateGameSessionInput, _ ...func(*gamelift.Options)) (*gamelift.CreateGameSessionOutput, error) {
	return &gamelift.CreateGameSessionOutput{}, nil
}

func (f *fakeGLClient) DescribeGameSessions(_ context.Context, _ *gamelift.DescribeGameSessionsInput, _ ...func(*gamelift.Options)) (*gamelift.DescribeGameSessionsOutput, error) {
	return &gamelift.DescribeGameSessionsOutput{}, nil
}

// stubFleetAPI replays scripted ListFleets and DescribeFleetAttributes pages
// so pagination handling can be tested hermetically.
type stubFleetAPI struct {
	listPages  []*gamelift.ListFleetsOutput
	descPages  []*gamelift.DescribeFleetAttributesOutput
	listErr    error
	descErr    error
	listCalls  int
	descCalls  int
	lastDescIn *gamelift.DescribeFleetAttributesInput
}

func (s *stubFleetAPI) ListFleets(ctx context.Context, params *gamelift.ListFleetsInput, optFns ...func(*gamelift.Options)) (*gamelift.ListFleetsOutput, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	page := s.listPages[s.listCalls]
	s.listCalls++
	return page, nil
}

func (s *stubFleetAPI) DescribeFleetAttributes(ctx context.Context, params *gamelift.DescribeFleetAttributesInput, optFns ...func(*gamelift.Options)) (*gamelift.DescribeFleetAttributesOutput, error) {
	if s.descErr != nil {
		return nil, s.descErr
	}
	page := s.descPages[s.descCalls]
	s.descCalls++
	s.lastDescIn = params
	return page, nil
}

func fleetAttr(name string) gltypes.FleetAttributes {
	return gltypes.FleetAttributes{
		Name:    aws.String(name),
		FleetId: aws.String("fleet-" + name),
		Status:  gltypes.FleetStatusActive,
	}
}

// TestRuntimeConfigurationAssignsDistinctPorts is the regression test for
// concurrent server processes sharing one UDP port: with
// maxConcurrentSessions=4 the runtime configuration must launch four
// ServerProcesses entries (ConcurrentExecutions=1 each) pinned to
// --port 7777..7780, not one entry whose four executions collide on 7777.
func TestRuntimeConfigurationAssignsDistinctPorts(t *testing.T) {
	got := runtimeConfiguration(7777, 4)
	if len(got.ServerProcesses) != 4 {
		t.Fatalf("ServerProcesses = %d, want 4 (one per concurrent session)", len(got.ServerProcesses))
	}
	for i, sp := range got.ServerProcesses {
		if want := fmt.Sprintf("--port %d", 7777+i); aws.ToString(sp.Parameters) != want {
			t.Errorf("ServerProcesses[%d].Parameters = %q, want %q", i, aws.ToString(sp.Parameters), want)
		}
		if aws.ToInt32(sp.ConcurrentExecutions) != 1 {
			t.Errorf("ServerProcesses[%d].ConcurrentExecutions = %v, want 1", i, sp.ConcurrentExecutions)
		}
		if aws.ToString(sp.LaunchPath) != ec2WrapperLaunchPath {
			t.Errorf("ServerProcesses[%d].LaunchPath = %q, want %q", i, aws.ToString(sp.LaunchPath), ec2WrapperLaunchPath)
		}
	}
}

// TestCreateFleetInputOpensPortRangeForConcurrency pins the matching inbound
// rule: N processes on [base, base+N-1] need the UDP permission to cover the
// whole contiguous block, not just the base port.
func TestCreateFleetInputOpensPortRangeForConcurrency(t *testing.T) {
	d := &Deployer{opts: DeployOptions{ServerPort: 7777, MaxConcurrentSessions: 3, AllowedCIDR: config.DefaultAllowedCIDR}}
	in := d.createFleetInput("build-1", "arn:role")

	if len(in.RuntimeConfiguration.ServerProcesses) != 3 {
		t.Fatalf("ServerProcesses = %d, want 3", len(in.RuntimeConfiguration.ServerProcesses))
	}
	perm := in.EC2InboundPermissions[0]
	if *perm.FromPort != 7777 || *perm.ToPort != 7779 {
		t.Errorf("port range = %d-%d, want 7777-7779", *perm.FromPort, *perm.ToPort)
	}
	if perm.Protocol != gltypes.IpProtocolUdp {
		t.Errorf("Protocol = %v, want UDP", perm.Protocol)
	}
}

// TestServerProcessCountClampToGameLiftLimit pins the resolution of
// maxConcurrentSessions: at least 1, and clamped to GameLift's per-fleet
// ServerProcesses limit because each process gets its own entry and port.
func TestServerProcessCountClampToGameLiftLimit(t *testing.T) {
	tests := []struct {
		name string
		set  int
		want int
	}{
		{"zero defaults to one process", 0, 1},
		{"negative defaults to one process", -3, 1},
		{"configured value passes through", 4, 4},
		{"above the GameLift limit clamps", 75, maxServerProcesses},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Deployer{opts: DeployOptions{MaxConcurrentSessions: tt.set}}
			if got := d.serverProcessCount(); got != tt.want {
				t.Errorf("serverProcessCount(%d) = %d, want %d", tt.set, got, tt.want)
			}
		})
	}
}

func TestCreateFleetInputDefaultCIDROpen(t *testing.T) {
	// The config layer (ResolvedAllowedCIDR) resolves the empty default
	// before DeployOptions is constructed; here the deployer just carries
	// the resolved value into the CreateFleet input.
	d := &Deployer{opts: DeployOptions{ServerPort: 7777, AllowedCIDR: config.DefaultAllowedCIDR}}
	in := d.createFleetInput("build-1", "arn:role")
	if len(in.EC2InboundPermissions) != 1 {
		t.Fatalf("EC2InboundPermissions = %d, want 1", len(in.EC2InboundPermissions))
	}
	perm := in.EC2InboundPermissions[0]
	if aws.ToString(perm.IpRange) != "0.0.0.0/0" {
		t.Errorf("IpRange = %q, want default 0.0.0.0/0", aws.ToString(perm.IpRange))
	}
	if *perm.FromPort != 7777 || *perm.ToPort != 7777 {
		t.Errorf("port range = %d-%d, want 7777-7777", *perm.FromPort, *perm.ToPort)
	}
	if perm.Protocol != gltypes.IpProtocolUdp {
		t.Errorf("Protocol = %v, want UDP", perm.Protocol)
	}
}

func TestCreateFleetInputUnsetCIDRUsesPublicDefault(t *testing.T) {
	// An unset AllowedCIDR (bypassing the config layer) must still land as
	// the public default on the CreateFleet input.
	d := &Deployer{opts: DeployOptions{ServerPort: 7777}}
	in := d.createFleetInput("build-1", "arn:role")
	if got := aws.ToString(in.EC2InboundPermissions[0].IpRange); got != config.DefaultAllowedCIDR {
		t.Errorf("IpRange = %q, want %s", got, config.DefaultAllowedCIDR)
	}
}

func TestCreateFleetInputAllowedCIDROverride(t *testing.T) {
	d := &Deployer{opts: DeployOptions{ServerPort: 7777, AllowedCIDR: "10.1.0.0/16"}}
	in := d.createFleetInput("build-1", "arn:role")
	if got := aws.ToString(in.EC2InboundPermissions[0].IpRange); got != "10.1.0.0/16" {
		t.Errorf("IpRange = %q, want 10.1.0.0/16", got)
	}
}

func TestOpenCIDRWarning(t *testing.T) {
	tests := []struct {
		name     string
		cidr     string
		port     int
		wantOpen bool
	}{
		{"unset CIDR falls back to public default", "", 7777, true},
		{"explicit public default", "0.0.0.0/0", 7777, true},
		{"restricted CIDR is silent", "10.1.0.0/16", 7777, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OpenCIDRWarning(tt.cidr, tt.port)
			if tt.wantOpen {
				if !strings.Contains(got, "WARNING:") || !strings.Contains(got, "0.0.0.0/0") || !strings.Contains(got, fmt.Sprintf("port %d", tt.port)) {
					t.Errorf("OpenCIDRWarning() = %q, want public-CIDR warning naming port %d", got, tt.port)
				}
			} else if got != "" {
				t.Errorf("OpenCIDRWarning() = %q, want empty for restricted CIDR", got)
			}
		})
	}
}

// TestCreateFleetPlumbsCIDRIntoCreateFleetInput drives the real CreateFleet
// (hermetically, via fakes): the open-CIDR case exercises the warning print
// and the restricted case the silent path, and both assert the configured
// CIDR lands on the submitted CreateFleetInput.
func TestCreateFleetPlumbsCIDRIntoCreateFleetInput(t *testing.T) {
	tests := []struct {
		name   string
		cidr   string
		wantIP string
	}{
		{"open default prints warning and is public", config.DefaultAllowedCIDR, "0.0.0.0/0"},
		{"restricted CIDR is silent and applied", "10.1.0.0/16", "10.1.0.0/16"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gl := &fakeGLClient{
				createFleetOut: &gamelift.CreateFleetOutput{
					FleetAttributes: &gltypes.FleetAttributes{FleetId: aws.String("fleet-1")},
				},
				describeOut: &gamelift.DescribeFleetAttributesOutput{
					FleetAttributes: []gltypes.FleetAttributes{fleetAttr("ludus-server")},
				},
			}
			iam := &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:aws:iam::123456789012:role/LudusGameLiftEC2FleetRole")}
			d := &Deployer{
				opts:      DeployOptions{FleetName: "ludus-server", ServerPort: 7777, AllowedCIDR: tt.cidr},
				glClient:  gl,
				iamClient: iam,
			}

			got, err := d.CreateFleet(context.Background(), "build-1")
			if err != nil {
				t.Fatalf("CreateFleet() error = %v", err)
			}
			if got.FleetID != "fleet-1" || got.BuildID != "build-1" {
				t.Errorf("CreateFleet() = %+v, want fleet-1/build-1", got)
			}
			if gl.createFleetCalls != 1 {
				t.Fatalf("CreateFleet calls = %d, want 1", gl.createFleetCalls)
			}
			perm := gl.createFleetInput.EC2InboundPermissions[0]
			if gotIP := aws.ToString(perm.IpRange); gotIP != tt.wantIP {
				t.Errorf("CreateFleet input IpRange = %q, want %s", gotIP, tt.wantIP)
			}
			if perm.Protocol != gltypes.IpProtocolUdp {
				t.Errorf("CreateFleet input Protocol = %v, want UDP", perm.Protocol)
			}
		})
	}
}

// TestCreateFleetInputDefaultsToOneProcessOnePort pins the default fleet
// shape: no concurrency configured means one server process on the single
// configured port, with the UDP permission still a one-port rule.
func TestCreateFleetInputDefaultsToOneProcessOnePort(t *testing.T) {
	d := &Deployer{opts: DeployOptions{ServerPort: 7777}}
	in := d.createFleetInput("build-1", "arn:role")

	if len(in.RuntimeConfiguration.ServerProcesses) != 1 {
		t.Fatalf("ServerProcesses = %d, want 1", len(in.RuntimeConfiguration.ServerProcesses))
	}
	sp := in.RuntimeConfiguration.ServerProcesses[0]
	if aws.ToString(sp.Parameters) != "--port 7777" || aws.ToInt32(sp.ConcurrentExecutions) != 1 {
		t.Errorf("ServerProcess = %q/%v, want --port 7777/1", aws.ToString(sp.Parameters), sp.ConcurrentExecutions)
	}
	perm := in.EC2InboundPermissions[0]
	if *perm.FromPort != 7777 || *perm.ToPort != 7777 {
		t.Errorf("port range = %d-%d, want 7777-7777", *perm.FromPort, *perm.ToPort)
	}
}

var errBoom = &smithy.GenericAPIError{Code: "InternalError", Message: "boom"}

// TestFleetDeletionTimeout is the regression test for the deletion-poll
// timeout being treated as success: ErrPollTimeout must surface as a
// contextual error so the fleet is reported as NOT deleted and the deployment
// state is kept for a safe retry.
func TestFleetDeletionTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"poll timeout becomes a contextual error", awsutil.ErrPollTimeout},
		{"nil stays nil", nil},
		{"other errors pass through unchanged", errBoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifyFleetDeletionTimeout(t, tt.err)
		})
	}
}

// verifyFleetDeletionTimeout checks the timeout-vs-success decision for one
// poll result.
func verifyFleetDeletionTimeout(t *testing.T, err error) {
	t.Helper()
	got := fleetDeletionTimeout(err, "fleet-1")
	switch err {
	case nil:
		if got != nil {
			t.Fatalf("fleetDeletionTimeout(nil) = %v, want nil", got)
		}
	case awsutil.ErrPollTimeout:
		verifyFleetTimeoutError(t, got)
	default:
		if got != err {
			t.Fatalf("fleetDeletionTimeout(%v) = %v, want the error passed through unchanged", err, got)
		}
	}
}

// verifyFleetTimeoutError checks the contextual timeout message.
func verifyFleetTimeoutError(t *testing.T, got error) {
	t.Helper()
	if got == nil || !strings.Contains(got.Error(), "fleet-1") || !strings.Contains(got.Error(), "timed out") {
		t.Fatalf("fleetDeletionTimeout(timeout) = %v, want contextual timeout naming the fleet", got)
	}
}

// TestWaitForFleetDeletionPropagatesPollError covers the hermetic slice of the
// deletion wait: a DescribeFleetAttributes failure surfaces immediately
// (instead of being swallowed into success).
func TestWaitForFleetDeletionPropagatesPollError(t *testing.T) {
	d := &Deployer{glClient: &fakeGLClient{describeErr: errBoom}}
	err := d.waitForFleetDeletion(context.Background(), "fleet-1")
	if err == nil || !strings.Contains(err.Error(), "polling fleet deletion") {
		t.Fatalf("waitForFleetDeletion() error = %v, want polling fleet deletion wrap", err)
	}
}

// TestDestroyStopsAfterFleetDeletionFailure pins the safe-retry contract: when
// the fleet itself can't be deleted, Destroy must stop before removing the
// dependent build, S3 object, and IAM role so a rerun finds a consistent
// deployment.
func TestDestroyStopsAfterFleetDeletionFailure(t *testing.T) {
	gl := &fakeGLClient{describeErr: errBoom}
	d := &Deployer{glClient: gl}

	err := d.Destroy(context.Background(), "fleet-1", "build-1", "ludus-builds-123", "ludus/key.zip")
	if err == nil {
		t.Fatal("Destroy() error = nil, want fleet deletion failure")
	}
	if gl.deleteFleetCalls != 1 {
		t.Errorf("DeleteFleet calls = %d, want 1", gl.deleteFleetCalls)
	}
	if gl.deleteBuildCalls != 0 {
		t.Errorf("DeleteBuild calls = %d, want 0 (build must survive a failed fleet deletion)", gl.deleteBuildCalls)
	}
}

func idList(prefix string, n int) []string {
	var ids []string
	for i := range n {
		ids = append(ids, prefix+string(rune('a'+i)))
	}
	return ids
}

// TestFindFleetByNameFollowsPagination pins the >16-fleet contract: lookup by
// name must follow ListFleets NextToken instead of matching only the first
// page of 16 IDs.
func TestFindFleetByNameFollowsPagination(t *testing.T) {
	stub := &stubFleetAPI{
		listPages: []*gamelift.ListFleetsOutput{
			{FleetIds: idList("p1-", 16), NextToken: aws.String("token-1")},
			{FleetIds: []string{"p2-ludus-server"}},
		},
		descPages: []*gamelift.DescribeFleetAttributesOutput{
			{FleetAttributes: []gltypes.FleetAttributes{fleetAttr("other")}},
			{FleetAttributes: []gltypes.FleetAttributes{fleetAttr("ludus-server")}},
		},
	}

	got, err := findFleetByName(context.Background(), stub, "ludus-server")
	if err != nil {
		t.Fatalf("findFleetByName() error = %v", err)
	}
	if aws.ToString(got.FleetId) != "fleet-ludus-server" {
		t.Errorf("findFleetByName() fleet id = %q, want fleet-ludus-server", aws.ToString(got.FleetId))
	}
	if stub.listCalls != 2 {
		t.Errorf("ListFleets calls = %d, want 2 (pagination)", stub.listCalls)
	}
}

// TestFindFleetByNameNotFound covers the exhausted-pagination miss.
func TestFindFleetByNameNotFound(t *testing.T) {
	stub := &stubFleetAPI{
		listPages: []*gamelift.ListFleetsOutput{
			{FleetIds: []string{"f1"}},
		},
		descPages: []*gamelift.DescribeFleetAttributesOutput{
			{FleetAttributes: []gltypes.FleetAttributes{fleetAttr("other")}},
		},
	}

	_, err := findFleetByName(context.Background(), stub, "ludus-server")
	if err == nil || !strings.Contains(err.Error(), "no fleet found") {
		t.Fatalf("findFleetByName() error = %v, want 'no fleet found'", err)
	}
}

// TestFindFleetByNameErrorPaths covers both API failure wraps.
func TestFindFleetByNameErrorPaths(t *testing.T) {
	tests := []struct {
		name     string
		stub     *stubFleetAPI
		wantText string
	}{
		{
			name:     "list error wraps",
			stub:     &stubFleetAPI{listErr: errBoom},
			wantText: "listing fleets",
		},
		{
			name: "describe error wraps",
			stub: &stubFleetAPI{
				listPages: []*gamelift.ListFleetsOutput{{FleetIds: []string{"f1"}}},
				descErr:   errBoom,
			},
			wantText: "describing fleet attributes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := findFleetByName(context.Background(), tt.stub, "ludus-server")
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("findFleetByName() error = %v, want %q wrap", err, tt.wantText)
			}
		})
	}
}
