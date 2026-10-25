package gamelift

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/jpvelasco/ludus/internal/state"
)

// recordGameLiftFleet writes a profile state recording a gamelift deployment
// of the given fleet ID, as adapter.Deploy does.
func recordGameLiftFleet(t *testing.T, fleetID string) {
	t.Helper()
	t.Chdir(t.TempDir())
	now := time.Now().UTC().Format(time.RFC3339)
	if err := state.UpdateFleet(&state.FleetState{FleetID: fleetID, Status: "ACTIVE", CreatedAt: now}); err != nil {
		t.Fatalf("UpdateFleet: %v", err)
	}
	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "gamelift", Status: "ACTIVE", DeployedAt: now,
	}); err != nil {
		t.Fatalf("UpdateDeploy: %v", err)
	}
}

func TestSelectFleet(t *testing.T) {
	tests := []struct {
		name         string
		recordedID   string
		fleetName    string
		candidates   []fleetCandidate
		wantID       string
		wantErrSub   string
		wantNilFleet bool
	}{
		{
			name:       "recorded ID wins over other fleets",
			recordedID: "fleet-2",
			fleetName:  "test-fleet",
			candidates: []fleetCandidate{
				{fleetID: "fleet-1", status: "ACTIVE"},
				{fleetID: "fleet-2", status: "ACTIVE"},
			},
			wantID: "fleet-2",
		},
		{
			name:       "recorded ID wins even when another fleet is owned",
			recordedID: "fleet-1",
			fleetName:  "test-fleet",
			candidates: []fleetCandidate{
				{fleetID: "fleet-1"},
				{fleetID: "fleet-2", ownedName: "test-fleet"},
			},
			wantID: "fleet-1",
		},
		{
			name:       "recorded ID absent from account is an error",
			recordedID: "fleet-gone",
			fleetName:  "test-fleet",
			candidates: []fleetCandidate{
				{fleetID: "fleet-1"},
				{fleetID: "fleet-2", ownedName: "test-fleet"},
			},
			wantErrSub: "recorded fleet \"fleet-gone\" not found",
		},
		{
			name:      "unique owned fleet wins among untagged fleets",
			fleetName: "test-fleet",
			candidates: []fleetCandidate{
				{fleetID: "fleet-1"},
				{fleetID: "fleet-2", ownedName: "test-fleet"},
			},
			wantID: "fleet-2",
		},
		{
			name:      "owned fleet wins even with several owned names present",
			fleetName: "mine",
			candidates: []fleetCandidate{
				{fleetID: "fleet-1", ownedName: "other"},
				{fleetID: "fleet-2", ownedName: "mine"},
			},
			wantID: "fleet-2",
		},
		{
			name:      "only fleet wins when untagged",
			fleetName: "test-fleet",
			candidates: []fleetCandidate{
				{fleetID: "fleet-1", status: "ACTIVE"},
			},
			wantID: "fleet-1",
		},
		{
			name:      "ambiguous untagged fleets error instead of first match",
			fleetName: "test-fleet",
			candidates: []fleetCandidate{
				{fleetID: "fleet-1"},
				{fleetID: "fleet-2"},
			},
			wantErrSub: "none is uniquely tagged",
		},
		{
			name:      "duplicate owned tags error instead of first match",
			fleetName: "test-fleet",
			candidates: []fleetCandidate{
				{fleetID: "fleet-1", ownedName: "test-fleet"},
				{fleetID: "fleet-2", ownedName: "test-fleet"},
			},
			wantErrSub: "none is uniquely tagged",
		},
		{
			name:         "no candidates yield nil fleet",
			fleetName:    "test-fleet",
			candidates:   nil,
			wantNilFleet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectFleet(tt.recordedID, tt.fleetName, tt.candidates)
			switch {
			case tt.wantErrSub != "":
				assertErrorContains(t, err, tt.wantErrSub)
				return
			case err != nil:
				t.Fatalf("selectFleet() error = %v", err)
			}
			if tt.wantNilFleet {
				if got != nil {
					t.Fatalf("selectFleet() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("selectFleet() = nil, want a fleet")
			}
			if got.fleetID != tt.wantID {
				t.Errorf("selectFleet() = %q, want %q", got.fleetID, tt.wantID)
			}
		})
	}
}

// TestGetFleetStatusPrefersRecordedFleet pins #716: when two fleets share the
// container group and the profile records the second, status (and session
// creation through it) must target the recorded fleet, not ContainerFleets[0].
func TestGetFleetStatusPrefersRecordedFleet(t *testing.T) {
	recordGameLiftFleet(t, "fleet-2")

	fleet1 := fleetPage("fleet-1", "ACTIVE", "")
	fleet2 := fleetPage("fleet-2", "ACTIVE", "")
	fleet1.ContainerFleets = append(fleet1.ContainerFleets, fleet2.ContainerFleets...)
	client := &fakeFleetClient{listOut: fleet1, tagsByARN: map[string]*gamelift.ListTagsForResourceOutput{
		"arn:aws:gamelift:us-east-1::fleet/fleet-1": {},
	}}

	got, err := newTestFleetDeployer(client, &fakeIAMClient{}).GetFleetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetFleetStatus() error = %v", err)
	}
	if got.FleetID != "fleet-2" {
		t.Errorf("GetFleetStatus() fleet = %q, want recorded fleet-2", got.FleetID)
	}
}

// TestGetFleetStatusTagScopedFallback covers discovery when no fleet ID is
// recorded: the unique fleet tagged with this deployer's ludus:fleet-name tag
// wins over an arbitrary first match.
func TestGetFleetStatusTagScopedFallback(t *testing.T) {
	t.Chdir(t.TempDir()) // no recorded state

	fleet1 := fleetPage("fleet-1", "ACTIVE", "")
	fleet2 := fleetPage("fleet-2", "ACTIVE", "")
	fleet1.ContainerFleets = append(fleet1.ContainerFleets, fleet2.ContainerFleets...)
	client := &fakeFleetClient{listOut: fleet1, tagsByARN: map[string]*gamelift.ListTagsForResourceOutput{
		"arn:aws:gamelift:us-east-1::fleet/fleet-1": {},
		"arn:aws:gamelift:us-east-1::fleet/fleet-2": fleetTagsOut("test-fleet"),
	}}

	got, err := newTestFleetDeployer(client, &fakeIAMClient{}).GetFleetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetFleetStatus() error = %v", err)
	}
	if got.FleetID != "fleet-2" {
		t.Errorf("GetFleetStatus() fleet = %q, want tagged fleet-2", got.FleetID)
	}
}

// TestGetFleetStatusAmbiguityError pins #716: with several unidentifiable
// fleets sharing the container group and no recorded ID, status must fail
// rather than silently report an arbitrary one.
func TestGetFleetStatusAmbiguityError(t *testing.T) {
	t.Chdir(t.TempDir()) // no recorded state

	page := fleetPage("fleet-1", "ACTIVE", "")
	page.ContainerFleets = append(page.ContainerFleets, gltypes.ContainerFleet{
		FleetId:  aws.String("fleet-2"),
		FleetArn: aws.String("arn:aws:gamelift:us-east-1::fleet/fleet-2"),
		Status:   gltypes.ContainerFleetStatusActive,
	})
	client := &fakeFleetClient{listOut: page}

	_, err := newTestFleetDeployer(client, &fakeIAMClient{}).GetFleetStatus(context.Background())
	assertErrorContains(t, err, "selecting fleet for container group test-group")
}

// TestGetFleetStatusFollowsPagination pins the pagination half of #716: the
// recorded fleet on a later page must be found, with the NextToken forwarded.
func TestGetFleetStatusFollowsPagination(t *testing.T) {
	recordGameLiftFleet(t, "fleet-2")

	page1 := fleetPage("fleet-1", "ACTIVE", "")
	page1.NextToken = aws.String("page-2")
	page2 := fleetPage("fleet-2", "ACTIVE", "")
	client := &fakeFleetClient{listPages: []*gamelift.ListContainerFleetsOutput{page1, page2}}

	got, err := newTestFleetDeployer(client, &fakeIAMClient{}).GetFleetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetFleetStatus() error = %v", err)
	}
	if got.FleetID != "fleet-2" {
		t.Errorf("GetFleetStatus() fleet = %q, want fleet-2 from the second page", got.FleetID)
	}
	if client.listCalls != 2 {
		t.Errorf("list calls = %d, want 2 (pagination must be followed)", client.listCalls)
	}
	if tok := aws.ToString(client.listInputs[1].NextToken); tok != "page-2" {
		t.Errorf("second list NextToken = %q, want %q", tok, "page-2")
	}
}

// TestFindContainerFleetIDPrefersRecordedID pins the teardown half of #716:
// destroy targets exactly the recorded fleet, without even listing.
func TestFindContainerFleetIDPrefersRecordedID(t *testing.T) {
	recordGameLiftFleet(t, "fleet-2")

	client := &fakeFleetClient{listOut: fleetSummary("fleet-1", "ACTIVE")}
	got, err := newTestFleetDeployer(client, &fakeIAMClient{}).findContainerFleetID(context.Background())
	if err != nil {
		t.Fatalf("findContainerFleetID() error = %v", err)
	}
	if got != "fleet-2" {
		t.Errorf("findContainerFleetID() = %q, want recorded fleet-2", got)
	}
	if client.listCalls != 0 {
		t.Errorf("list calls = %d, want 0 (recorded fleet must not require discovery)", client.listCalls)
	}
}

// TestFindContainerFleetIDTagScopedFallback covers legacy deployments with no
// recorded fleet: teardown picks the unique owned fleet, not the first.
func TestFindContainerFleetIDTagScopedFallback(t *testing.T) {
	t.Chdir(t.TempDir()) // no recorded state

	fleet1 := fleetPage("fleet-1", "ACTIVE", "")
	fleet2 := fleetPage("fleet-2", "ACTIVE", "")
	fleet1.ContainerFleets = append(fleet1.ContainerFleets, fleet2.ContainerFleets...)
	client := &fakeFleetClient{listOut: fleet1, tagsByARN: map[string]*gamelift.ListTagsForResourceOutput{
		"arn:aws:gamelift:us-east-1::fleet/fleet-1": {},
		"arn:aws:gamelift:us-east-1::fleet/fleet-2": fleetTagsOut("test-fleet"),
	}}

	got, err := newTestFleetDeployer(client, &fakeIAMClient{}).findContainerFleetID(context.Background())
	if err != nil {
		t.Fatalf("findContainerFleetID() error = %v", err)
	}
	if got != "fleet-2" {
		t.Errorf("findContainerFleetID() = %q, want tagged fleet-2", got)
	}
}

func TestFindContainerFleetIDAmbiguityError(t *testing.T) {
	t.Chdir(t.TempDir()) // no recorded state

	page := fleetPage("fleet-1", "ACTIVE", "")
	page.ContainerFleets = append(page.ContainerFleets, gltypes.ContainerFleet{
		FleetId:  aws.String("fleet-2"),
		FleetArn: aws.String("arn:aws:gamelift:us-east-1::fleet/fleet-2"),
		Status:   gltypes.ContainerFleetStatusActive,
	})
	client := &fakeFleetClient{listOut: page}

	_, err := newTestFleetDeployer(client, &fakeIAMClient{}).findContainerFleetID(context.Background())
	assertErrorContains(t, err, "selecting fleet for container group test-group")
}

// TestDeleteFleetTargetsRecordedFleet drives the full deleteFleet flow with a
// recorded fleet that differs from the only other fleet sharing the group.
func TestDeleteFleetTargetsRecordedFleet(t *testing.T) {
	recordGameLiftFleet(t, "fleet-2")

	fleet := &fakeFleetClient{
		listOut:     fleetSummary("fleet-1", "ACTIVE"),
		describeErr: &cgdAPIError{code: "NotFoundException"},
	}
	d := newTestFleetDeployer(fleet, &fakeIAMClient{})

	if err := d.deleteFleet(context.Background()); err != nil {
		t.Fatalf("deleteFleet() error = %v", err)
	}
	if len(fleet.deletedFleetIDs) != 1 || fleet.deletedFleetIDs[0] != "fleet-2" {
		t.Errorf("deleted fleets = %v, want exactly [fleet-2]", fleet.deletedFleetIDs)
	}
}

// TestRecordedFleetID covers the recorded-ID scoping rules: only fleet IDs
// recorded by a gamelift deployment may be adopted.
func TestRecordedFleetID(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		want    string
		wantErr bool
	}{
		{
			name: "gamelift deployment returns fleet ID",
			setup: func(t *testing.T) {
				recordGameLiftFleet(t, "fleet-1")
			},
			want: "fleet-1",
		},
		{
			name: "other targets fleet ID is ignored",
			setup: func(t *testing.T) {
				t.Chdir(t.TempDir())
				now := time.Now().UTC().Format(time.RFC3339)
				if err := state.UpdateFleet(&state.FleetState{FleetID: "stack-fleet", Status: "ACTIVE", CreatedAt: now}); err != nil {
					t.Fatal(err)
				}
				if err := state.UpdateDeploy(&state.DeployState{
					TargetName: "stack", Status: "ACTIVE", DeployedAt: now,
				}); err != nil {
					t.Fatal(err)
				}
			},
			want: "",
		},
		{
			name: "no state returns empty",
			setup: func(t *testing.T) {
				t.Chdir(t.TempDir())
			},
			want: "",
		},
		{
			name: "unreadable state returns empty",
			setup: func(t *testing.T) {
				t.Chdir(t.TempDir())
				if err := os.MkdirAll(".ludus", 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(".ludus", "state.json"), []byte("{not json"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			if got := recordedFleetID(); got != tt.want {
				t.Errorf("recordedFleetID() = %q, want %q", got, tt.want)
			}
		})
	}
}
