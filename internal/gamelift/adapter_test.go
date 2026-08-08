package gamelift

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

// newAdapterDeployer builds a Deployer whose fleet/cgd/iam clients are fakes
// pre-wired for a successful end-to-end deploy, so adapter tests only need to
// override the piece they exercise.
func newAdapterDeployer() *Deployer {
	fleet := &fakeFleetClient{
		createOut: &gamelift.CreateContainerFleetOutput{
			ContainerFleet: &gltypes.ContainerFleet{FleetId: aws.String("fleet-adapter")},
		},
		describeOut: &gamelift.DescribeContainerFleetOutput{
			ContainerFleet: &gltypes.ContainerFleet{Status: gltypes.ContainerFleetStatusActive},
		},
		listOut: fleetSummary("fleet-adapter", "ACTIVE"),
		sessionOut: &gamelift.CreateGameSessionOutput{
			GameSession: &gltypes.GameSession{
				GameSessionId: aws.String("sess-adapter"),
				IpAddress:     aws.String("203.0.113.9"),
				Port:          aws.Int32(7777),
			},
		},
		describeSOut: &gamelift.DescribeGameSessionsOutput{
			GameSessions: []gltypes.GameSession{{Status: gltypes.GameSessionStatusActive}},
		},
	}
	cgd := &fakeCGDClient{
		createResults: []cgdCreateResult{{out: createCGDOutput("arn:cgd-adapter")}},
		describeResults: []cgdDescribeResult{
			{out: readyCGDOutput("arn:cgd-adapter")},
			{out: readyCGDOutput("arn:cgd-adapter")},
		},
	}
	iam := &fakeIAMClient{getRoleOut: iamRoleOutput("arn:role")}
	return &Deployer{
		opts: DeployOptions{
			FleetName:          "adapter-fleet",
			InstanceType:       "c5.large",
			ContainerGroupName: "adapter-group",
			ServerPort:         7777,
			Tags:               map[string]string{"ManagedBy": "ludus"},
		},
		glClient:            fleet,
		cgdClient:           cgd,
		iamClient:           iam,
		iamPropagationDelay: 0,
	}
}

func newAdapter(t *testing.T) *TargetAdapter {
	t.Helper()
	t.Chdir(t.TempDir())
	return NewTargetAdapter(newAdapterDeployer())
}

func TestAdapterBasics(t *testing.T) {
	d := &Deployer{}
	a := NewTargetAdapter(d)

	if a.Name() != "gamelift" {
		t.Errorf("Name() = %q, want gamelift", a.Name())
	}
	if a.Deployer() != d {
		t.Error("Deployer() does not return the wrapped deployer")
	}

	caps := a.Capabilities()
	want := deploy.Capabilities{
		NeedsContainerBuild: true,
		NeedsContainerPush:  true,
		SupportsSession:     true,
		SupportsDeploy:      true,
		SupportsDestroy:     true,
	}
	if caps != want {
		t.Errorf("Capabilities() = %+v, want %+v", caps, want)
	}
}

func TestAdapterDeployWritesState(t *testing.T) {
	a := newAdapter(t)

	result, err := a.Deploy(context.Background(), deploy.DeployInput{})
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	want := deploy.DeployResult{TargetName: "gamelift", Status: "ACTIVE", Detail: "fleet fleet-adapter"}
	if *result != want {
		t.Errorf("Deploy() result = %+v, want %+v", result, want)
	}

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load(): %v", err)
	}
	assertDeployStateWritten(t, s)
}

func assertDeployStateWritten(t *testing.T, s *state.State) {
	t.Helper()
	if s.Fleet == nil || s.Fleet.FleetID != "fleet-adapter" || s.Fleet.Status != "ACTIVE" {
		t.Errorf("fleet state = %+v, want fleet-adapter/ACTIVE", s.Fleet)
	}
	if s.Deploy == nil || s.Deploy.TargetName != "gamelift" || s.Deploy.Status != "ACTIVE" || s.Deploy.Detail != "fleet fleet-adapter" {
		t.Errorf("deploy state = %+v, want gamelift/ACTIVE", s.Deploy)
	}
}

func TestAdapterDeployWarnsWhenStateUnwritable(t *testing.T) {
	a := newAdapter(t)
	// Make the project state path unwritable: state.json as a directory.
	if err := os.MkdirAll(".ludus", 0755); err != nil {
		t.Fatalf("MkdirAll(.ludus): %v", err)
	}
	if err := os.Mkdir(filepath.Join(".ludus", "state.json"), 0755); err != nil {
		t.Fatalf("Mkdir(state.json): %v", err)
	}

	result, err := a.Deploy(context.Background(), deploy.DeployInput{})
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if result.Status != "ACTIVE" {
		t.Errorf("Deploy() result = %+v, want ACTIVE", result)
	}
}

func TestAdapterDeployCgdError(t *testing.T) {
	a := newAdapter(t)
	a.deployer.cgdClient = &fakeCGDClient{
		createResults: []cgdCreateResult{{err: &cgdAPIError{code: "AccessDeniedException"}}},
	}

	if _, err := a.Deploy(context.Background(), deploy.DeployInput{}); err == nil {
		t.Fatal("Deploy() error = nil, want cgd error")
	}
}

func TestAdapterDeployFleetError(t *testing.T) {
	a := newAdapter(t)
	a.deployer.cgdClient = &fakeCGDClient{
		createResults: []cgdCreateResult{{out: createCGDOutput("arn:cgd")}},
		describeResults: []cgdDescribeResult{
			{out: readyCGDOutput("arn:cgd")},
		},
	}
	a.deployer.glClient = &fakeFleetClient{
		createErr: &cgdAPIError{code: "LimitExceededException"},
	}

	if _, err := a.Deploy(context.Background(), deploy.DeployInput{}); err == nil {
		t.Fatal("Deploy() error = nil, want fleet error")
	}
}

func TestAdapterStatus(t *testing.T) {
	t.Run("active when fleet found", func(t *testing.T) {
		a := newAdapter(t)

		got, err := a.Status(context.Background())
		if err != nil {
			t.Fatalf("Status() error = %v", err)
		}
		if got.TargetName != "gamelift" || got.Status != "active" || got.Detail != "fleet-adapter (ACTIVE)" {
			t.Errorf("Status() = %+v", got)
		}
	})

	t.Run("not deployed when no fleet", func(t *testing.T) {
		a := newAdapter(t)
		a.deployer.glClient = &fakeFleetClient{listOut: &gamelift.ListContainerFleetsOutput{}}

		got, err := a.Status(context.Background())
		if err != nil {
			t.Fatalf("Status() error = %v", err)
		}
		if got.Status != "not_deployed" || got.Detail != "no fleet found" {
			t.Errorf("Status() = %+v, want not_deployed", got)
		}
	})
}

func TestAdapterDestroy(t *testing.T) {
	t.Run("tears down and clears state", func(t *testing.T) {
		a := newAdapter(t)
		fleet := a.deployer.glClient.(*fakeFleetClient)
		fleet.listOut = &gamelift.ListContainerFleetsOutput{}

		if err := a.Destroy(context.Background()); err != nil {
			t.Fatalf("Destroy() error = %v", err)
		}
		if fleet.listCalls != 1 {
			t.Errorf("list calls = %d, want 1", fleet.listCalls)
		}
	})

	t.Run("propagates deployer error", func(t *testing.T) {
		a := newAdapter(t)
		a.deployer.glClient = &fakeFleetClient{listErr: &cgdAPIError{code: "AccessDeniedException"}}

		if err := a.Destroy(context.Background()); err == nil {
			t.Fatal("Destroy() error = nil, want fleet error")
		}
	})
}

func TestAdapterCreateSession(t *testing.T) {
	t.Run("creates session and saves state", func(t *testing.T) {
		a := newAdapter(t)

		info, err := a.CreateSession(context.Background(), 8)
		if err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}
		if info.SessionID != "sess-adapter" || info.IPAddress != "203.0.113.9" || info.Port != 7777 {
			t.Errorf("CreateSession() = %+v", info)
		}

		s, err := state.Load()
		if err != nil {
			t.Fatalf("state.Load(): %v", err)
		}
		if s.Session == nil || s.Session.SessionID != "sess-adapter" {
			t.Errorf("session state = %+v, want sess-adapter", s.Session)
		}
	})

	t.Run("fails when no fleet", func(t *testing.T) {
		a := newAdapter(t)
		a.deployer.glClient = &fakeFleetClient{listOut: &gamelift.ListContainerFleetsOutput{}}

		_, err := a.CreateSession(context.Background(), 4)
		assertErrorContains(t, err, "finding fleet")
	})

	t.Run("propagates session create error", func(t *testing.T) {
		a := newAdapter(t)
		a.deployer.glClient = &fakeFleetClient{
			listOut:    fleetSummary("fleet-adapter", "ACTIVE"),
			sessionErr: &cgdAPIError{code: "LimitExceededException"},
		}

		_, err := a.CreateSession(context.Background(), 4)
		assertErrorContains(t, err, "creating game session")
	})
}

func TestAdapterDescribeSession(t *testing.T) {
	a := newAdapter(t)

	status, err := a.DescribeSession(context.Background(), "sess-adapter")
	if err != nil {
		t.Fatalf("DescribeSession() error = %v", err)
	}
	if status != "ACTIVE" {
		t.Errorf("DescribeSession() = %q, want ACTIVE", status)
	}
}
