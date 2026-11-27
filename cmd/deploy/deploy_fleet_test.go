package deploy

import (
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/gamelift"
	"github.com/jpvelasco/ludus/internal/state"
)

func assertFleet(t *testing.T, fleet *state.FleetState, wantFleetID, wantStatus string) {
	t.Helper()
	if fleet == nil {
		t.Fatal("expected Fleet to be set")
		return
	}
	if fleet.FleetID != wantFleetID {
		t.Errorf("FleetID = %q, want %q", fleet.FleetID, wantFleetID)
	}
	if fleet.Status != wantStatus {
		t.Errorf("Fleet.Status = %q, want %q", fleet.Status, wantStatus)
	}
	if fleet.CreatedAt == "" {
		t.Error("Fleet.CreatedAt should be set")
	}
}

func assertDeploy(t *testing.T, deploy *state.DeployState, wantFleetID, wantStatus string) {
	t.Helper()
	if deploy == nil {
		t.Fatal("expected Deploy to be set")
		return
	}
	if deploy.TargetName != "gamelift" {
		t.Errorf("Deploy.TargetName = %q, want %q", deploy.TargetName, "gamelift")
	}
	if deploy.Status != wantStatus {
		t.Errorf("Deploy.Status = %q, want %q", deploy.Status, wantStatus)
	}
	detail := "fleet " + wantFleetID
	if deploy.Detail != detail {
		t.Errorf("Deploy.Detail = %q, want %q", deploy.Detail, detail)
	}
	if deploy.DeployedAt == "" {
		t.Error("Deploy.DeployedAt should be set")
	}
}

func TestRecordFleetDeployState(t *testing.T) {
	t.Chdir(t.TempDir())

	gamelift.RecordFleetState(&gamelift.FleetStatus{FleetID: "fleet-abc123", Status: "ACTIVE"}, io.Discard)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	assertFleet(t, s.Fleet, "fleet-abc123", "ACTIVE")
	assertDeploy(t, s.Deploy, "fleet-abc123", "ACTIVE")
}

func TestRecordFleetDeployState_OverwritesPreviousState(t *testing.T) {
	t.Chdir(t.TempDir())

	gamelift.RecordFleetState(&gamelift.FleetStatus{FleetID: "fleet-old", Status: "CREATING"}, io.Discard)
	gamelift.RecordFleetState(&gamelift.FleetStatus{FleetID: "fleet-new", Status: "ACTIVE"}, io.Discard)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	assertFleet(t, s.Fleet, "fleet-new", "ACTIVE")
	assertDeploy(t, s.Deploy, "fleet-new", "ACTIVE")
}

// TestPrintFleetPlan covers the #778 dry-run plan: every resolved value the
// operator needs to review before a billable create is printed.
func TestPrintFleetPlan(t *testing.T) {
	d := gamelift.NewDeployer(gamelift.DeployOptions{
		ImageURI:              "123456789.dkr.ecr.us-east-1.amazonaws.com/ludus-server:latest",
		FleetName:             "test-fleet",
		InstanceType:          "c6i.large",
		ContainerGroupName:    "test-group",
		IAMRoleName:           "LudusGameLiftContainerFleetRole",
		MaxConcurrentSessions: 3,
		Tags:                  map[string]string{"ManagedBy": "ludus", "Project": "Lyra"},
	}, aws.Config{})

	out := captureStdout(func() { printFleetPlan(d.Plan()) })

	for _, want := range []string{
		"123456789.dkr.ecr.us-east-1.amazonaws.com/ludus-server:latest",
		"test-group",
		"test-fleet",
		"LudusGameLiftContainerFleetRole",
		"c6i.large",
		"3",
		"0.0.0.0/0",
		"ManagedBy",
		"Project",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestPrintFleetOutcome(t *testing.T) {
	tests := []struct {
		name        string
		action      gamelift.DeployAction
		want        string
		wantInbound bool
	}{
		{name: "created", action: gamelift.ActionCreated, want: "Fleet deployed: fleet-1", wantInbound: true},
		{name: "updated", action: gamelift.ActionUpdated, want: "Fleet updated in place: fleet-1"},
		{name: "unchanged", action: gamelift.ActionUnchanged, want: "Fleet unchanged: fleet-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals.SetGlobals(t, testGameliftCfg())
			out := captureStdout(func() {
				printFleetOutcome(&gamelift.FleetStatus{FleetID: "fleet-1", Status: "ACTIVE", Action: tt.action})
			})
			if !strings.Contains(out, tt.want) {
				t.Errorf("output = %q, want %q", out, tt.want)
			}
			if got := strings.Contains(out, "Inbound:"); got != tt.wantInbound {
				t.Errorf("inbound line printed = %v, want %v (inbound rules only describe a new fleet)", got, tt.wantInbound)
			}
		})
	}
}
