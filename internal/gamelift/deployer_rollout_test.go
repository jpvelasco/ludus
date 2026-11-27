package gamelift

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

func TestUpdateContainerGroupDefinitionInput(t *testing.T) {
	d := chooseFixture()
	latest := describedDefinition(d, "arn:cgd:3", 3, "sha256:a")

	got := updateContainerGroupDefinitionInput(d.containerGroupDefinitionInput(), latest, "ludus deploy fleet now")

	checks := map[string]bool{
		"name":                  aws.ToString(got.Name) == "test-group",
		"based on the latest":   aws.ToInt32(got.SourceVersionNumber) == 3,
		"image":                 aws.ToString(got.GameServerContainerDefinition.ImageUri) == testECRHost+"/ludus-server:latest",
		"sdk version":           aws.ToString(got.GameServerContainerDefinition.ServerSdkVersion) == "5.4.0",
		"port":                  aws.ToInt32(got.GameServerContainerDefinition.PortConfiguration.ContainerPortRanges[0].FromPort) == 7777,
		"memory limit":          aws.ToInt32(got.TotalMemoryLimitMebibytes) == 1024,
		"vcpu limit":            aws.ToFloat64(got.TotalVcpuLimit) == 1.0,
		"version description":   aws.ToString(got.VersionDescription) == "ludus deploy fleet now",
		"operating system kept": got.OperatingSystem == "",
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("updateContainerGroupDefinitionInput(): %s is wrong in %+v", name, got)
		}
	}
}

func TestUpdateContainerFleetInput(t *testing.T) {
	got := updateContainerFleetInput("fleet-1", "arn:cgd:2")

	if aws.ToString(got.FleetId) != "fleet-1" || aws.ToString(got.GameServerContainerGroupDefinitionName) != "arn:cgd:2" {
		t.Errorf("updateContainerFleetInput() = %+v, want fleet-1 pinned to arn:cgd:2", got)
	}
	cfg := got.DeploymentConfiguration
	if cfg.ProtectionStrategy != gltypes.DeploymentProtectionStrategyWithProtection ||
		cfg.ImpairmentStrategy != gltypes.DeploymentImpairmentStrategyRollback {
		t.Errorf("DeploymentConfiguration = %+v, want WITH_PROTECTION and ROLLBACK", cfg)
	}
}

func TestRolloutTrackerObserve(t *testing.T) {
	const prev, next = "dep-0", "dep-1"
	tests := []struct {
		name     string
		snap     fleetDeploymentSnapshot
		wantDone bool
		wantErr  string
	}{
		{name: "no deployment visible yet", snap: fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusUpdating}},
		{name: "still showing the previous deployment", snap: fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusActive, deploymentID: prev, deployStatus: gltypes.DeploymentStatusComplete}},
		{name: "new deployment in progress", snap: fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusUpdating, deploymentID: next, deployStatus: gltypes.DeploymentStatusInProgress}},
		{name: "complete and active", snap: fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusActive, deploymentID: next, deployStatus: gltypes.DeploymentStatusComplete}, wantDone: true},
		{name: "complete but fleet still updating", snap: fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusUpdating, deploymentID: next, deployStatus: gltypes.DeploymentStatusComplete}},
		{name: "impaired", snap: fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusUpdating, deploymentID: next, deployStatus: gltypes.DeploymentStatusImpaired}, wantErr: "IMPAIRED"},
		{name: "rollback in progress", snap: fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusUpdating, deploymentID: next, deployStatus: gltypes.DeploymentStatusRollbackInProgress}, wantErr: "previous image"},
		{name: "rollback complete", snap: fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusActive, deploymentID: next, deployStatus: gltypes.DeploymentStatusRollbackComplete}, wantErr: "failed validation"},
		{name: "cancelled", snap: fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusUpdating, deploymentID: next, deployStatus: gltypes.DeploymentStatusCancelled}, wantErr: "cancelled"},
		{name: "fleet expired", snap: fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusExpired, deploymentID: next}, wantErr: "EXPIRED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := &rolloutTracker{prevDeploymentID: prev}
			done, err := tracker.observe(tt.snap)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil || done != tt.wantDone {
				t.Errorf("observe() = %v, %v; want done=%v", done, err, tt.wantDone)
			}
		})
	}
}

// An update that changes nothing starts no deployment; on an ACTIVE fleet the
// tracker must give up rather than poll for the full wait window.
func TestRolloutTrackerGivesUpWhenNoDeploymentStarts(t *testing.T) {
	tracker := &rolloutTracker{prevDeploymentID: "dep-0"}
	idle := fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusActive, deploymentID: "dep-0", deployStatus: gltypes.DeploymentStatusComplete}

	for i := 1; i < maxIdleRolloutPolls; i++ {
		if _, err := tracker.observe(idle); err != nil {
			t.Fatalf("poll %d: error = %v, want patience", i, err)
		}
	}
	_, err := tracker.observe(idle)
	assertErrorContains(t, err, "no new deployment started")
}

// A fleet that has already left ACTIVE is mid-update, so the idle count
// restarts: the new deployment is about to register.
func TestRolloutTrackerResetsIdleCountWhileUpdating(t *testing.T) {
	tracker := &rolloutTracker{prevDeploymentID: "dep-0", idlePolls: maxIdleRolloutPolls - 1}
	updating := fleetDeploymentSnapshot{fleetStatus: gltypes.ContainerFleetStatusUpdating, deploymentID: "dep-0"}

	if _, err := tracker.observe(updating); err != nil {
		t.Fatalf("observe() error = %v, want none while the fleet is UPDATING", err)
	}
	if tracker.idlePolls != 0 {
		t.Errorf("idlePolls = %d, want 0", tracker.idlePolls)
	}
}

func TestDeploymentFailureNamesTheDeploymentAndNextStep(t *testing.T) {
	for _, status := range []gltypes.DeploymentStatus{
		gltypes.DeploymentStatusImpaired,
		gltypes.DeploymentStatusRollbackInProgress,
		gltypes.DeploymentStatusRollbackComplete,
	} {
		err := deploymentFailure("dep-9", status)
		if !strings.Contains(err.Error(), "dep-9") || !strings.Contains(err.Error(), "GAME_SERVER_CONTAINER_GROUP_CRASHED") {
			t.Errorf("deploymentFailure(%s) = %v, want deployment id and where to look", status, err)
		}
	}
}
