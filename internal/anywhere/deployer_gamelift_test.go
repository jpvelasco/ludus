package anywhere

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/jpvelasco/ludus/internal/runner"
)

// newTestDeployer builds a Deployer wired to the given fake GameLift client.
func newTestDeployer(fake *fakeGameLift, opts DeployOptions) *Deployer {
	return &Deployer{opts: opts, glClient: fake, Runner: runner.NewRunner(false, true)}
}

func TestCreateLocation_CreatesNew(t *testing.T) {
	fake := &fakeGameLift{}
	d := newTestDeployer(fake, DeployOptions{LocationName: "ludus-dev", Region: "us-west-2"})

	arn, created, err := d.CreateLocation(context.Background())
	if err != nil {
		t.Fatalf("CreateLocation: %v", err)
	}
	if !fake.createdLocation {
		t.Error("expected CreateLocation API call")
	}
	if !created {
		t.Error("created should be true for a freshly created location")
	}
	if arn == "" {
		t.Error("expected a location ARN")
	}
}

func TestCreateLocation_ReusesOnConflict(t *testing.T) {
	// A ConflictException means the location already exists — CreateLocation must
	// tolerate it and report created=false so callers don't later delete a
	// location they didn't create. This guards the rollback safety property.
	fake := &fakeGameLift{createLocationErr: &gltypes.ConflictException{Message: aws.String("exists")}}
	d := newTestDeployer(fake, DeployOptions{LocationName: "custom-ludus-dev", Region: "us-west-2"})

	arn, created, err := d.CreateLocation(context.Background())
	if err != nil {
		t.Fatalf("CreateLocation should tolerate conflict: %v", err)
	}
	if created {
		t.Error("created must be false when reusing an existing location")
	}
	if arn == "" {
		t.Error("expected a synthesized ARN for the reused location")
	}
}

func TestCreateLocation_PropagatesError(t *testing.T) {
	fake := &fakeGameLift{createLocationErr: errors.New("boom")}
	d := newTestDeployer(fake, DeployOptions{LocationName: "loc"})
	if _, _, err := d.CreateLocation(context.Background()); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestCreateFleet(t *testing.T) {
	fake := &fakeGameLift{}
	d := newTestDeployer(fake, DeployOptions{FleetName: "ludus-fleet"})

	fleetID, fleetARN, err := d.CreateFleet(context.Background(), "custom-loc")
	if err != nil {
		t.Fatalf("CreateFleet: %v", err)
	}
	if !fake.createdFleet {
		t.Error("expected CreateFleet API call")
	}
	if fleetID == "" || fleetARN == "" {
		t.Errorf("expected fleet id+arn, got %q / %q", fleetID, fleetARN)
	}
}

func TestDestroy_AggregatesErrors(t *testing.T) {
	// A failing teardown step must surface as an error (not be silently
	// swallowed), so callers like rollback and `deploy destroy` can report it.
	fake := &fakeGameLift{deleteFleetErr: errors.New("fleet stuck")}
	d := newTestDeployer(fake, DeployOptions{})

	err := d.Destroy(context.Background(), "fleet-1", "compute-1", "custom-loc", 0)
	if err == nil {
		t.Fatal("expected Destroy to return the fleet-deletion error")
	}
	// Other steps still run despite the fleet error.
	if !fake.deregisteredCompute || !fake.deletedLocation {
		t.Error("Destroy must attempt all teardown steps even when one fails")
	}
}

func TestDestroy_AllSucceed(t *testing.T) {
	fake := &fakeGameLift{}
	d := newTestDeployer(fake, DeployOptions{})
	if err := d.Destroy(context.Background(), "fleet-1", "compute-1", "custom-loc", 0); err != nil {
		t.Fatalf("Destroy should succeed: %v", err)
	}
	if !fake.deletedFleet || !fake.deregisteredCompute || !fake.deletedLocation {
		t.Error("Destroy must delete fleet, deregister compute, and delete location")
	}
}

func TestRollbackLaunchFailure_SurfacesTeardownError(t *testing.T) {
	// When teardown itself fails during rollback, the warning branch runs (the
	// rollback is best-effort and must not panic or block the launch error).
	fake := &fakeGameLift{deleteFleetErr: errors.New("boom")}
	d := newTestDeployer(fake, DeployOptions{LocationName: "custom-loc"})
	a := NewTargetAdapter(d)

	a.rollbackLaunchFailure(context.Background(), "fleet-1", "compute-1", true)

	if !fake.deletedFleet {
		t.Error("rollback should still attempt fleet deletion")
	}
}

func TestGetFleetStatus(t *testing.T) {
	fake := &fakeGameLift{fleetStatus: gltypes.FleetStatusActive}
	d := newTestDeployer(fake, DeployOptions{})

	status, err := d.GetFleetStatus(context.Background(), "fleet-1")
	if err != nil {
		t.Fatalf("GetFleetStatus: %v", err)
	}
	if status != string(gltypes.FleetStatusActive) {
		t.Errorf("status = %q, want ACTIVE", status)
	}
}

func TestDeployerSession(t *testing.T) {
	fake := &fakeGameLift{}
	d := newTestDeployer(fake, DeployOptions{})

	info, err := d.CreateGameSession(context.Background(), "fleet-1", "custom-loc", 8)
	if err != nil {
		t.Fatalf("CreateGameSession: %v", err)
	}
	if !fake.createdSession || info.SessionID == "" {
		t.Errorf("expected a created session, got %+v (called=%v)", info, fake.createdSession)
	}

	status, err := d.DescribeGameSession(context.Background(), "sess-test")
	if err != nil {
		t.Fatalf("DescribeGameSession: %v", err)
	}
	if status != string(gltypes.GameSessionStatusActive) {
		t.Errorf("status = %q, want ACTIVE", status)
	}
}
