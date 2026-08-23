package ec2fleet

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/aws/smithy-go"
)

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
