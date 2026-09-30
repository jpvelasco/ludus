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
	return &gamelift.DeleteFleetOutput{}, nil
}

func (f *fakeGLClient) CreateBuild(_ context.Context, _ *gamelift.CreateBuildInput, _ ...func(*gamelift.Options)) (*gamelift.CreateBuildOutput, error) {
	return &gamelift.CreateBuildOutput{}, nil
}

func (f *fakeGLClient) DescribeBuild(_ context.Context, _ *gamelift.DescribeBuildInput, _ ...func(*gamelift.Options)) (*gamelift.DescribeBuildOutput, error) {
	return &gamelift.DescribeBuildOutput{}, nil
}

func (f *fakeGLClient) DeleteBuild(_ context.Context, _ *gamelift.DeleteBuildInput, _ ...func(*gamelift.Options)) (*gamelift.DeleteBuildOutput, error) {
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

func TestRuntimeConfigurationAppliesMaxConcurrentSessions(t *testing.T) {
	d := &Deployer{opts: DeployOptions{ServerPort: 7777, MaxConcurrentSessions: 4}}
	got := d.runtimeConfiguration()
	if len(got.ServerProcesses) != 1 {
		t.Fatalf("ServerProcesses = %d, want 1", len(got.ServerProcesses))
	}
	if got.ServerProcesses[0].ConcurrentExecutions == nil || *got.ServerProcesses[0].ConcurrentExecutions != 4 {
		t.Errorf("ConcurrentExecutions = %v, want 4", got.ServerProcesses[0].ConcurrentExecutions)
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

func TestRuntimeConfigurationDefaultsMaxConcurrentSessions(t *testing.T) {
	d := &Deployer{opts: DeployOptions{ServerPort: 7777}}
	got := d.runtimeConfiguration()
	if got.ServerProcesses[0].ConcurrentExecutions == nil || *got.ServerProcesses[0].ConcurrentExecutions != 1 {
		t.Errorf("ConcurrentExecutions = %v, want 1 default", got.ServerProcesses[0].ConcurrentExecutions)
	}
}

var errBoom = &smithy.GenericAPIError{Code: "InternalError", Message: "boom"}

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
