package ec2fleet

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/state"
)

func TestDestroyPlanUsesEC2ArchiveCoordinates(t *testing.T) {
	st := &state.State{
		Fleet: &state.FleetState{FleetID: "foreign"},
		EC2Fleet: &state.EC2FleetState{
			FleetID: "ec2-fleet", BuildID: "build", S3Bucket: "recorded-bucket", S3Key: "archive/build.zip",
		},
	}
	plan, err := PlanDestroy(testRoleOptions(), st)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"ec2-fleet", "build", "s3://recorded-bucket/archive/build.zip", "LudusGameLiftEC2FleetRole"} {
		if plan.Items[i].Identifier != want {
			t.Errorf("item %d = %+v, want %s", i, plan.Items[i], want)
		}
	}
	if !strings.Contains(strings.Join(plan.Items[3].Conditions, "\n"), "ludus:target=\"ec2\"") {
		t.Fatal("EC2 target guard missing")
	}
}

func TestDestroyPlanMissingEC2StateRemainsUnresolved(t *testing.T) {
	for _, st := range []*state.State{{}, {EC2Fleet: &state.EC2FleetState{S3Bucket: "bucket-only"}}} {
		plan, err := PlanDestroy(testRoleOptions(), st)
		if err != nil {
			t.Fatal(err)
		}
		for i, want := range []string{"unresolved", "skip", "skip", "conditional"} {
			if plan.Items[i].Decision != want {
				t.Errorf("item %d = %+v, want %s", i, plan.Items[i], want)
			}
		}
	}
}

func TestDestroyPlanRejectsInvalidEC2Identity(t *testing.T) {
	if _, err := PlanDestroy(DeployOptions{}, &state.State{}); err == nil {
		t.Fatal("invalid expected identity accepted")
	}
}
