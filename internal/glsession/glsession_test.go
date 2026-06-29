package glsession

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

// fakeSessionAPI implements API, capturing the input passed to CreateGameSession
// and returning canned responses or injected errors. This lets the tests drive
// the real Create/Describe functions rather than a parallel reimplementation.
type fakeSessionAPI struct {
	createInput *gamelift.CreateGameSessionInput
	createOut   *gamelift.CreateGameSessionOutput
	createErr   error
	describeOut *gamelift.DescribeGameSessionsOutput
	describeErr error
}

func (f *fakeSessionAPI) CreateGameSession(_ context.Context, in *gamelift.CreateGameSessionInput, _ ...func(*gamelift.Options)) (*gamelift.CreateGameSessionOutput, error) {
	f.createInput = in
	return f.createOut, f.createErr
}

func (f *fakeSessionAPI) DescribeGameSessions(_ context.Context, _ *gamelift.DescribeGameSessionsInput, _ ...func(*gamelift.Options)) (*gamelift.DescribeGameSessionsOutput, error) {
	return f.describeOut, f.describeErr
}

func sessionOutput(id, ip string, port int32) *gamelift.CreateGameSessionOutput {
	return &gamelift.CreateGameSessionOutput{
		GameSession: &gltypes.GameSession{
			GameSessionId: aws.String(id),
			IpAddress:     aws.String(ip),
			Port:          aws.Int32(port),
		},
	}
}

func TestCreate_ForwardsLocationAndParsesOutput(t *testing.T) {
	fake := &fakeSessionAPI{createOut: sessionOutput("sess-1", "203.0.113.5", 7777)}

	info, err := Create(context.Background(), fake, "fleet-1", "custom-loc", 8)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.SessionID != "sess-1" || info.IPAddress != "203.0.113.5" || info.Port != 7777 {
		t.Errorf("unexpected session info: %+v", info)
	}
	// Anywhere fleets require the location be included in the request.
	if aws.ToString(fake.createInput.Location) != "custom-loc" {
		t.Errorf("Location = %v, want custom-loc", fake.createInput.Location)
	}
	if aws.ToString(fake.createInput.FleetId) != "fleet-1" {
		t.Errorf("FleetId = %v, want fleet-1", fake.createInput.FleetId)
	}
	if aws.ToInt32(fake.createInput.MaximumPlayerSessionCount) != 8 {
		t.Errorf("MaximumPlayerSessionCount = %v, want 8", fake.createInput.MaximumPlayerSessionCount)
	}
}

func TestCreate_OmitsEmptyLocation(t *testing.T) {
	fake := &fakeSessionAPI{createOut: sessionOutput("s", "1.2.3.4", 1)}
	if _, err := Create(context.Background(), fake, "fleet-1", "", 4); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if fake.createInput.Location != nil {
		t.Errorf("expected nil Location, got %q", aws.ToString(fake.createInput.Location))
	}
}

func TestCreate_WrapsAPIError(t *testing.T) {
	fake := &fakeSessionAPI{createErr: errors.New("boom")}
	if _, err := Create(context.Background(), fake, "f", "", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestDescribe(t *testing.T) {
	t.Run("returns status", func(t *testing.T) {
		fake := &fakeSessionAPI{describeOut: &gamelift.DescribeGameSessionsOutput{
			GameSessions: []gltypes.GameSession{{Status: gltypes.GameSessionStatusActivating}},
		}}
		status, err := Describe(context.Background(), fake, "sess-1")
		if err != nil {
			t.Fatalf("Describe: %v", err)
		}
		if status != "ACTIVATING" {
			t.Errorf("status = %q, want ACTIVATING", status)
		}
	})

	t.Run("errors when session not found", func(t *testing.T) {
		fake := &fakeSessionAPI{describeOut: &gamelift.DescribeGameSessionsOutput{}}
		if _, err := Describe(context.Background(), fake, "missing"); err == nil {
			t.Fatal("expected not-found error")
		}
	})

	t.Run("wraps API error", func(t *testing.T) {
		fake := &fakeSessionAPI{describeErr: errors.New("boom")}
		if _, err := Describe(context.Background(), fake, "s"); err == nil {
			t.Fatal("expected error")
		}
	})
}
