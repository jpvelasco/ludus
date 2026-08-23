package gamelift

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

// fakeFleetClient implements fleetAPI with canned responses and call
// capture, letting tests drive the real Deployer fleet lifecycle.
type fakeFleetClient struct {
	createOut    *gamelift.CreateContainerFleetOutput
	createErr    error
	describeOut  *gamelift.DescribeContainerFleetOutput
	describeErr  error
	listOut      *gamelift.ListContainerFleetsOutput
	listErr      error
	deleteErr    error
	sessionOut   *gamelift.CreateGameSessionOutput
	sessionErr   error
	describeSOut *gamelift.DescribeGameSessionsOutput
	describeSErr error

	createFleetCalls int
	listCalls        int
	describeCalls    int
	deleteCalls      int

	createFleetInput *gamelift.CreateContainerFleetInput
}

func (f *fakeFleetClient) CreateContainerFleet(_ context.Context, in *gamelift.CreateContainerFleetInput, _ ...func(*gamelift.Options)) (*gamelift.CreateContainerFleetOutput, error) {
	f.createFleetCalls++
	f.createFleetInput = in
	return f.createOut, f.createErr
}

func (f *fakeFleetClient) DescribeContainerFleet(_ context.Context, _ *gamelift.DescribeContainerFleetInput, _ ...func(*gamelift.Options)) (*gamelift.DescribeContainerFleetOutput, error) {
	f.describeCalls++
	return f.describeOut, f.describeErr
}

func (f *fakeFleetClient) ListContainerFleets(_ context.Context, _ *gamelift.ListContainerFleetsInput, _ ...func(*gamelift.Options)) (*gamelift.ListContainerFleetsOutput, error) {
	f.listCalls++
	return f.listOut, f.listErr
}

func (f *fakeFleetClient) DeleteContainerFleet(_ context.Context, _ *gamelift.DeleteContainerFleetInput, _ ...func(*gamelift.Options)) (*gamelift.DeleteContainerFleetOutput, error) {
	f.deleteCalls++
	return &gamelift.DeleteContainerFleetOutput{}, f.deleteErr
}

func (f *fakeFleetClient) CreateGameSession(_ context.Context, _ *gamelift.CreateGameSessionInput, _ ...func(*gamelift.Options)) (*gamelift.CreateGameSessionOutput, error) {
	return f.sessionOut, f.sessionErr
}

func (f *fakeFleetClient) DescribeGameSessions(_ context.Context, _ *gamelift.DescribeGameSessionsInput, _ ...func(*gamelift.Options)) (*gamelift.DescribeGameSessionsOutput, error) {
	return f.describeSOut, f.describeSErr
}

func newTestFleetDeployer(client *fakeFleetClient, iam *fakeIAMClient) *Deployer {
	return &Deployer{
		opts: DeployOptions{
			FleetName:          "test-fleet",
			InstanceType:       "c5.large",
			ContainerGroupName: "test-group",
			ServerPort:         7777,
			Tags:               map[string]string{"ManagedBy": "ludus"},
		},
		glClient:            client,
		cgdClient:           &fakeCGDClient{},
		iamClient:           iam,
		iamPropagationDelay: 0,
	}
}

func fleetSummary(fleetID, status string) *gamelift.ListContainerFleetsOutput {
	return &gamelift.ListContainerFleetsOutput{
		ContainerFleets: []gltypes.ContainerFleet{
			{FleetId: aws.String(fleetID), Status: gltypes.ContainerFleetStatus(status)},
		},
	}
}

func TestGetFleetStatus(t *testing.T) {
	tests := []struct {
		name       string
		listOut    *gamelift.ListContainerFleetsOutput
		listErr    error
		wantErrSub string
	}{
		{
			name:    "returns first fleet",
			listOut: fleetSummary("fleet-1", "ACTIVE"),
		},
		{
			name:       "wraps list error",
			listErr:    &cgdAPIError{code: "AccessDeniedException"},
			wantErrSub: "listing fleets",
		},
		{
			name:       "no fleets found",
			listOut:    &gamelift.ListContainerFleetsOutput{},
			wantErrSub: "no fleets found for container group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeFleetClient{listOut: tt.listOut, listErr: tt.listErr}
			d := newTestFleetDeployer(client, &fakeIAMClient{})

			got, err := d.GetFleetStatus(context.Background())
			if tt.wantErrSub != "" {
				assertErrorContains(t, err, tt.wantErrSub)
				return
			}
			if err != nil {
				t.Fatalf("GetFleetStatus() error = %v", err)
			}
			if got.FleetID != "fleet-1" || got.Status != "ACTIVE" {
				t.Errorf("GetFleetStatus() = %+v, want fleet-1/ACTIVE", got)
			}
		})
	}
}

func TestWaitForContainerFleetActive(t *testing.T) {
	t.Run("marks status and returns when active", func(t *testing.T) {
		client := &fakeFleetClient{describeOut: &gamelift.DescribeContainerFleetOutput{
			ContainerFleet: &gltypes.ContainerFleet{Status: gltypes.ContainerFleetStatusActive},
		}}
		result := &FleetStatus{}
		err := newTestFleetDeployer(client, &fakeIAMClient{}).waitForContainerFleetActive(context.Background(), "fleet-1", result)
		if err != nil {
			t.Fatalf("waitForContainerFleetActive() error = %v", err)
		}
		if result.Status != "ACTIVE" {
			t.Errorf("result.Status = %q, want ACTIVE", result.Status)
		}
	})

	t.Run("wraps describe error", func(t *testing.T) {
		client := &fakeFleetClient{describeErr: &cgdAPIError{code: "InternalServiceException"}}
		err := newTestFleetDeployer(client, &fakeIAMClient{}).waitForContainerFleetActive(context.Background(), "fleet-1", &FleetStatus{})
		assertErrorContains(t, err, "polling fleet status")
	})

	// EXPIRED is a terminal provisioning failure (e.g. bad image reference).
	// The wait must fail fast with the status name instead of polling the
	// full 30-minute window; the short ctx timeout bounds the old-code hang
	// this test would otherwise incur.
	t.Run("fails fast on terminal EXPIRED", func(t *testing.T) {
		client := &fakeFleetClient{describeOut: &gamelift.DescribeContainerFleetOutput{
			ContainerFleet: &gltypes.ContainerFleet{Status: gltypes.ContainerFleetStatusExpired},
		}}
		result := &FleetStatus{}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := newTestFleetDeployer(client, &fakeIAMClient{}).waitForContainerFleetActive(ctx, "fleet-1", result)

		if err == nil || !strings.Contains(err.Error(), "EXPIRED") {
			t.Fatalf("waitForContainerFleetActive() error = %v, want terminal EXPIRED failure", err)
		}
		if client.describeCalls != 1 {
			t.Errorf("describe calls = %d, want 1 (fail fast)", client.describeCalls)
		}
	})
}

func TestCreateFleetErrors(t *testing.T) {
	tests := []struct {
		name       string
		fleet      *fakeFleetClient
		iam        *fakeIAMClient
		wantErrSub string
	}{
		{
			name: "iam role error propagates",
			iam:  &fakeIAMClient{getRoleErr: &cgdAPIError{code: "AccessDeniedException"}, createRoleErr: &cgdAPIError{code: "AccessDeniedException"}},
		},
		{
			name: "create fleet error",
			fleet: &fakeFleetClient{
				createErr: &cgdAPIError{code: "LimitExceededException"},
			},
			iam:        &fakeIAMClient{getRoleOut: iamRoleOutput("arn:role")},
			wantErrSub: "creating container fleet",
		},
		{
			name: "wait error returns partial result",
			fleet: &fakeFleetClient{
				createOut: &gamelift.CreateContainerFleetOutput{
					ContainerFleet: &gltypes.ContainerFleet{FleetId: aws.String("fleet-created")},
				},
				describeErr: &cgdAPIError{code: "InternalServiceException"},
			},
			iam:        &fakeIAMClient{getRoleOut: iamRoleOutput("arn:role")},
			wantErrSub: "polling fleet status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fleet == nil {
				tt.fleet = &fakeFleetClient{}
			}
			if tt.iam == nil {
				tt.iam = &fakeIAMClient{}
			}
			_, err := newTestFleetDeployer(tt.fleet, tt.iam).CreateFleet(context.Background(), "arn:cgd")
			assertErrorContains(t, err, tt.wantErrSub)
		})
	}
}

func TestCreateFleetSuccess(t *testing.T) {
	fleet := &fakeFleetClient{
		createOut: &gamelift.CreateContainerFleetOutput{
			ContainerFleet: &gltypes.ContainerFleet{FleetId: aws.String("fleet-created")},
		},
		describeOut: &gamelift.DescribeContainerFleetOutput{
			ContainerFleet: &gltypes.ContainerFleet{Status: gltypes.ContainerFleetStatusActive},
		},
	}
	iam := &fakeIAMClient{getRoleOut: iamRoleOutput("arn:aws:iam::123456789012:role/LudusGameLiftContainerFleetRole")}

	got, err := newTestFleetDeployer(fleet, iam).CreateFleet(context.Background(), "arn:cgd")
	if err != nil {
		t.Fatalf("CreateFleet() error = %v", err)
	}
	want := FleetStatus{
		FleetID:              "fleet-created",
		FleetName:            "test-fleet",
		Status:               "ACTIVE",
		InstanceType:         "c5.large",
		ContainerGroupDefARN: "arn:cgd",
	}
	if *got != want {
		t.Errorf("CreateFleet() = %+v, want %+v", got, want)
	}
	if gotRole := aws.ToString(fleet.createFleetInput.FleetRoleArn); gotRole != "arn:aws:iam::123456789012:role/LudusGameLiftContainerFleetRole" {
		t.Errorf("FleetRoleArn = %q, want test role ARN", gotRole)
	}
}

func TestDeleteFleet(t *testing.T) {
	tests := []struct {
		name       string
		fleet      *fakeFleetClient
		wantErrSub string
		wantLists  int
		wantDel    int
	}{
		{
			name:      "success",
			fleet:     &fakeFleetClient{listOut: fleetSummary("fleet-1", "ACTIVE"), describeErr: &cgdAPIError{code: "NotFoundException"}},
			wantLists: 1,
			wantDel:   1,
		},
		{
			name:      "no fleet found skips",
			fleet:     &fakeFleetClient{listOut: &gamelift.ListContainerFleetsOutput{}},
			wantLists: 1,
		},
		{
			name:       "list error",
			fleet:      &fakeFleetClient{listErr: &cgdAPIError{code: "AccessDeniedException"}},
			wantErrSub: "listing fleets",
			wantLists:  1,
		},
		{
			name:      "already deleted treated as success",
			fleet:     &fakeFleetClient{listOut: fleetSummary("fleet-1", "ACTIVE"), deleteErr: &cgdAPIError{code: "NotFoundException"}, describeErr: &cgdAPIError{code: "NotFoundException"}},
			wantLists: 1,
			wantDel:   1,
		},
		{
			name:       "delete error",
			fleet:      &fakeFleetClient{listOut: fleetSummary("fleet-1", "ACTIVE"), deleteErr: &cgdAPIError{code: "AccessDeniedException"}},
			wantErrSub: "deleting fleet",
			wantLists:  1,
			wantDel:    1,
		},
		{
			name:       "deletion poll error",
			fleet:      &fakeFleetClient{listOut: fleetSummary("fleet-1", "ACTIVE"), describeErr: &cgdAPIError{code: "InternalServiceException"}},
			wantErrSub: "polling fleet deletion",
			wantLists:  1,
			wantDel:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestFleetDeployer(tt.fleet, &fakeIAMClient{})
			err := d.deleteFleet(context.Background())
			if tt.wantErrSub != "" {
				assertErrorContains(t, err, tt.wantErrSub)
			} else if err != nil {
				t.Fatalf("deleteFleet() error = %v", err)
			}
			if tt.fleet.listCalls != tt.wantLists || tt.fleet.deleteCalls != tt.wantDel {
				t.Errorf("deleteFleet() calls = list:%d delete:%d, want list:%d delete:%d",
					tt.fleet.listCalls, tt.fleet.deleteCalls, tt.wantLists, tt.wantDel)
			}
		})
	}
}

func TestFindContainerFleetID(t *testing.T) {
	tests := []struct {
		name       string
		listOut    *gamelift.ListContainerFleetsOutput
		listErr    error
		wantID     string
		wantErrSub string
	}{
		{name: "returns first fleet ID", listOut: fleetSummary("fleet-1", "ACTIVE"), wantID: "fleet-1"},
		{name: "empty list returns empty ID", listOut: &gamelift.ListContainerFleetsOutput{}},
		{name: "not found returns empty ID", listErr: &cgdAPIError{code: "NotFoundException"}},
		{name: "wraps list error", listErr: &cgdAPIError{code: "AccessDeniedException"}, wantErrSub: "listing fleets"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeFleetClient{listOut: tt.listOut, listErr: tt.listErr}
			got, err := newTestFleetDeployer(client, &fakeIAMClient{}).findContainerFleetID(context.Background())
			if tt.wantErrSub != "" {
				assertErrorContains(t, err, tt.wantErrSub)
				return
			}
			if err != nil {
				t.Fatalf("findContainerFleetID() error = %v", err)
			}
			if got != tt.wantID {
				t.Errorf("findContainerFleetID() = %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestDestroyTearsDownAllResources(t *testing.T) {
	fleet := &fakeFleetClient{
		listOut:     fleetSummary("fleet-1", "ACTIVE"),
		describeErr: &cgdAPIError{code: "NotFoundException"},
	}
	cgd := &fakeCGDClient{}
	iam := &fakeIAMClient{}

	d := &Deployer{
		opts:                DeployOptions{ContainerGroupName: "test-group"},
		glClient:            fleet,
		cgdClient:           cgd,
		iamClient:           iam,
		iamPropagationDelay: 0,
	}

	if err := d.Destroy(context.Background()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if fleet.deleteCalls != 1 {
		t.Errorf("fleet delete calls = %d, want 1", fleet.deleteCalls)
	}
	if cgd.deleteCalls != 1 {
		t.Errorf("cgd delete calls = %d, want 1", cgd.deleteCalls)
	}
	if iam.detachCalls != 1 || iam.deleteRoleCalls != 1 {
		t.Errorf("iam calls = detach:%d delete:%d, want 1/1", iam.detachCalls, iam.deleteRoleCalls)
	}
}

func TestDestroyStopsAtFleetError(t *testing.T) {
	fleet := &fakeFleetClient{listErr: &cgdAPIError{code: "AccessDeniedException"}}
	cgd := &fakeCGDClient{}
	iam := &fakeIAMClient{}

	d := &Deployer{
		opts:                DeployOptions{ContainerGroupName: "test-group"},
		glClient:            fleet,
		cgdClient:           cgd,
		iamClient:           iam,
		iamPropagationDelay: 0,
	}

	err := d.Destroy(context.Background())
	assertErrorContains(t, err, "listing fleets")
	if cgd.deleteCalls != 0 || iam.detachCalls != 0 {
		t.Error("destroy continued after fleet error")
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want error containing %q", err, want)
	}
}
