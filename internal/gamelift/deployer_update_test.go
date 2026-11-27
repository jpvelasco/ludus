package gamelift

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

const (
	runningARN = "arn:aws:gamelift:us-west-2:123456789012:containergroupdefinition/test-group:1"
	newARN     = "arn:aws:gamelift:us-west-2:123456789012:containergroupdefinition/test-group:2"
)

// updateScenario wires a Deployer to fakes for a fleet that already exists
// ("fleet-1", owned by this deployment, ACTIVE, on definition version 1).
type updateScenario struct {
	d     *Deployer
	fleet *fakeFleetClient
	cgd   *fakeCGDClient
	ecr   *fakeECRClient
}

// fleetOutput is a DescribeContainerFleet result for fleet-1.
func fleetOutput(status gltypes.ContainerFleetStatus, definitionARN, deploymentID string) *gamelift.DescribeContainerFleetOutput {
	return &gamelift.DescribeContainerFleetOutput{ContainerFleet: &gltypes.ContainerFleet{
		FleetId:                               aws.String("fleet-1"),
		Status:                                status,
		InstanceType:                          aws.String("c5.large"),
		GameServerContainerGroupDefinitionArn: aws.String(definitionARN),
		DeploymentDetails:                     &gltypes.DeploymentDetails{LatestDeploymentId: aws.String(deploymentID)},
	}}
}

func deploymentOutput(status gltypes.DeploymentStatus) *gamelift.DescribeFleetDeploymentOutput {
	return &gamelift.DescribeFleetDeploymentOutput{FleetDeployment: &gltypes.FleetDeployment{DeploymentStatus: status}}
}

// newUpdateScenario builds the scenario. running and latest are what
// DescribeContainerGroupDefinition returns for the fleet's version and the
// newest version; afterUpdate is the READY version a wait then sees. digest is
// the image's current ECR digest.
func newUpdateScenario(t *testing.T, running, latest, afterUpdate *gltypes.ContainerGroupDefinition, digest string) *updateScenario {
	t.Helper()
	t.Chdir(t.TempDir())

	fleet := &fakeFleetClient{
		listOut:       fleetPage("fleet-1", "ACTIVE", ""),
		tagsOut:       fleetTagsOut("test-fleet"),
		deploymentOut: deploymentOutput(gltypes.DeploymentStatusComplete),
		describeQueue: []*gamelift.DescribeContainerFleetOutput{
			fleetOutput(gltypes.ContainerFleetStatusActive, aws.ToString(running.ContainerGroupDefinitionArn), "dep-0"),
			fleetOutput(gltypes.ContainerFleetStatusActive, newARN, "dep-1"),
		},
	}
	cgd := &fakeCGDClient{
		describeResults: []cgdDescribeResult{
			{out: describeOutput(running)},
			{out: describeOutput(latest)},
			{out: describeOutput(afterUpdate)},
		},
		updateResults: []cgdUpdateResult{{out: &gamelift.UpdateContainerGroupDefinitionOutput{ContainerGroupDefinition: afterUpdate}}},
	}
	ecr := &fakeECRClient{out: ecrDigestOut(digest)}

	d := newTestFleetDeployer(fleet, &fakeIAMClient{})
	d.opts.ImageURI = testECRHost + "/ludus-server:latest"
	d.cgdClient, d.ecrClient = cgd, ecr
	return &updateScenario{d: d, fleet: fleet, cgd: cgd, ecr: ecr}
}

func describeOutput(def *gltypes.ContainerGroupDefinition) *gamelift.DescribeContainerGroupDefinitionOutput {
	return &gamelift.DescribeContainerGroupDefinitionOutput{ContainerGroupDefinition: def}
}

// scenarioDefs returns the definitions a scenario starts from: the version the
// fleet runs matches the image at digest, and the new version is the same
// image re-snapshotted at newDigest.
func scenarioDefs(d *Deployer, digest, newDigest string) (running, next *gltypes.ContainerGroupDefinition) {
	return describedDefinition(d, runningARN, 1, digest), describedDefinition(d, newARN, 2, newDigest)
}

func assertNoMutations(t *testing.T, s *updateScenario) {
	t.Helper()
	if s.cgd.createCalls+s.cgd.updateCalls+s.cgd.deleteCalls != 0 || s.fleet.createFleetCalls+s.fleet.updateCalls+s.fleet.deleteCalls != 0 {
		t.Errorf("mutating calls made: cgd create/update/delete %d/%d/%d, fleet create/update/delete %d/%d/%d; want none",
			s.cgd.createCalls, s.cgd.updateCalls, s.cgd.deleteCalls, s.fleet.createFleetCalls, s.fleet.updateCalls, s.fleet.deleteCalls)
	}
}

func TestDeployFleetUpdatesInPlaceWhenTagIsRepushed(t *testing.T) {
	probe := chooseFixture()
	running, next := scenarioDefs(probe, "sha256:old", "sha256:new")
	s := newUpdateScenario(t, running, running, next, "sha256:new")
	s.d.opts.ContainerGroupName = "test-group"

	got, err := s.d.DeployFleet(context.Background(), false)
	if err != nil {
		t.Fatalf("DeployFleet() error = %v", err)
	}

	if got.Action != ActionUpdated || got.FleetID != "fleet-1" || got.ContainerGroupDefARN != newARN {
		t.Errorf("DeployFleet() = %+v, want fleet-1 updated onto %s", got, newARN)
	}
	assertInPlaceCalls(t, s)
}

// assertInPlaceCalls pins the update's AWS call shape: a new version cut from
// version 1, the same fleet pinned to it, and no fleet or definition replaced.
func assertInPlaceCalls(t *testing.T, s *updateScenario) {
	t.Helper()
	if s.fleet.createFleetCalls != 0 || s.cgd.deleteCalls != 0 {
		t.Errorf("created %d fleets and deleted %d definitions; an update must do neither", s.fleet.createFleetCalls, s.cgd.deleteCalls)
	}
	if s.cgd.updateCalls != 1 || aws.ToInt32(s.cgd.updateInputs[0].SourceVersionNumber) != 1 {
		t.Errorf("UpdateContainerGroupDefinition calls = %d, want 1 from version 1", s.cgd.updateCalls)
	}
	in := s.fleet.updateInputs[0]
	if aws.ToString(in.FleetId) != "fleet-1" || aws.ToString(in.GameServerContainerGroupDefinitionName) != newARN {
		t.Errorf("UpdateContainerFleet input = %+v, want fleet-1 pinned to %s", in, newARN)
	}
}

func TestDeployFleetReportsUnchangedWithoutMutating(t *testing.T) {
	probe := chooseFixture()
	running, _ := scenarioDefs(probe, "sha256:same", "sha256:same")
	s := newUpdateScenario(t, running, running, running, "sha256:same")

	got, err := s.d.DeployFleet(context.Background(), false)
	if err != nil {
		t.Fatalf("DeployFleet() error = %v", err)
	}
	if got.Action != ActionUnchanged || got.FleetID != "fleet-1" {
		t.Errorf("DeployFleet() = %+v, want fleet-1 unchanged", got)
	}
	assertNoMutations(t, s)
}

func TestDeployFleetForceCutsANewVersion(t *testing.T) {
	probe := chooseFixture()
	running, next := scenarioDefs(probe, "sha256:same", "sha256:same")
	s := newUpdateScenario(t, running, running, next, "sha256:same")

	got, err := s.d.DeployFleet(context.Background(), true)
	if err != nil {
		t.Fatalf("DeployFleet(force) error = %v", err)
	}
	if got.Action != ActionUpdated || s.cgd.updateCalls != 1 || s.fleet.updateCalls != 1 {
		t.Errorf("force: action %q, definition updates %d, fleet updates %d; want updated/1/1", got.Action, s.cgd.updateCalls, s.fleet.updateCalls)
	}
}

// After a rollout that rolled back, the latest version matches the image but
// the fleet is still on the old one: only the fleet needs to move.
func TestDeployFleetRollsOntoMatchingLatestWithoutNewVersion(t *testing.T) {
	probe := chooseFixture()
	_, latest := scenarioDefs(probe, "sha256:a", "sha256:b")
	running := staleDefinition(probe, runningARN, 1, "sha256:a")
	s := newUpdateScenario(t, running, latest, latest, "sha256:b")

	got, err := s.d.DeployFleet(context.Background(), false)
	if err != nil {
		t.Fatalf("DeployFleet() error = %v", err)
	}
	if got.Action != ActionUpdated || s.cgd.updateCalls != 0 || s.fleet.updateCalls != 1 {
		t.Errorf("roll-only: action %q, definition updates %d, fleet updates %d; want updated/0/1", got.Action, s.cgd.updateCalls, s.fleet.updateCalls)
	}
}

func TestDeployFleetFailsWhenTheDeploymentRollsBack(t *testing.T) {
	probe := chooseFixture()
	running, next := scenarioDefs(probe, "sha256:old", "sha256:new")
	s := newUpdateScenario(t, running, running, next, "sha256:new")
	s.fleet.deploymentOut = deploymentOutput(gltypes.DeploymentStatusRollbackInProgress)

	got, err := s.d.DeployFleet(context.Background(), false)

	assertErrorContains(t, err, "failed validation")
	if got != nil {
		t.Errorf("DeployFleet() = %+v, want nil result on a failed rollout", got)
	}
}

func TestDeployFleetRefusesAFleetThatIsNotActive(t *testing.T) {
	probe := chooseFixture()
	running, next := scenarioDefs(probe, "sha256:old", "sha256:new")
	s := newUpdateScenario(t, running, running, next, "sha256:new")
	s.fleet.describeQueue = []*gamelift.DescribeContainerFleetOutput{fleetOutput(gltypes.ContainerFleetStatusUpdating, runningARN, "dep-0")}

	_, err := s.d.DeployFleet(context.Background(), false)

	assertErrorContains(t, err, "wait for it to become ACTIVE")
	assertNoMutations(t, s)
}

func TestDeployFleetRefusesADefinitionSharedWithAnotherDeployment(t *testing.T) {
	probe := chooseFixture()
	running, next := scenarioDefs(probe, "sha256:old", "sha256:new")
	s := newUpdateScenario(t, running, running, next, "sha256:new")
	s.fleet.listOut = &gamelift.ListContainerFleetsOutput{ContainerFleets: []gltypes.ContainerFleet{
		{FleetId: aws.String("fleet-1"), FleetArn: aws.String("arn:fleet-1"), Status: gltypes.ContainerFleetStatusActive},
		{FleetId: aws.String("fleet-other"), FleetArn: aws.String("arn:fleet-other"), Status: gltypes.ContainerFleetStatusActive},
	}}
	s.fleet.tagsByARN = map[string]*gamelift.ListTagsForResourceOutput{
		"arn:fleet-1":     fleetTagsOut("test-fleet"),
		"arn:fleet-other": fleetTagsOut("someone-elses-fleet"),
	}

	_, err := s.d.DeployFleet(context.Background(), false)

	assertErrorContains(t, err, "also used by fleet-other")
	assertNoMutations(t, s)
}

// newCreatePathDeployer builds a deployer whose fleet client reports the
// given fleets, with fakes ready for a successful first-time create.
func newCreatePathDeployer(t *testing.T, listOut *gamelift.ListContainerFleetsOutput, tagsOut *gamelift.ListTagsForResourceOutput) (*Deployer, *fakeFleetClient) {
	t.Helper()
	t.Chdir(t.TempDir())
	fleet := &fakeFleetClient{
		listOut:     listOut,
		tagsOut:     tagsOut,
		createOut:   &gamelift.CreateContainerFleetOutput{ContainerFleet: &gltypes.ContainerFleet{FleetId: aws.String("fleet-new")}},
		describeOut: fleetOutput(gltypes.ContainerFleetStatusActive, runningARN, "dep-0"),
	}
	d := newTestFleetDeployer(fleet, &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:role")})
	d.cgdClient = &fakeCGDClient{
		createResults:   []cgdCreateResult{{out: createCGDOutput(runningARN)}},
		describeResults: []cgdDescribeResult{{out: readyCGDOutput(runningARN)}},
	}
	return d, fleet
}

func TestDeployFleetCreatesWhenThisDeploymentHasNoFleet(t *testing.T) {
	d, fleet := newCreatePathDeployer(t, &gamelift.ListContainerFleetsOutput{}, nil)

	got, err := d.DeployFleet(context.Background(), false)
	if err != nil {
		t.Fatalf("DeployFleet() error = %v", err)
	}
	if got.Action != ActionCreated || got.FleetID != "fleet-new" || fleet.createFleetCalls != 1 {
		t.Errorf("DeployFleet() = %+v after %d fleet creates, want fleet-new created once", got, fleet.createFleetCalls)
	}
}

// A fleet tagged for another deployment is not this deployment's to modify,
// even when it is the only fleet sharing the definition: deploy creates its own.
func TestDeployFleetIgnoresAnotherDeploymentsFleet(t *testing.T) {
	d, fleet := newCreatePathDeployer(t, fleetPage("fleet-other", "ACTIVE", ""), fleetTagsOut("someone-elses-fleet"))

	got, err := d.DeployFleet(context.Background(), false)
	if err != nil {
		t.Fatalf("DeployFleet() error = %v", err)
	}
	if got.Action != ActionCreated || got.FleetID != "fleet-new" || fleet.updateCalls != 0 {
		t.Errorf("DeployFleet() = %+v with %d fleet updates, want a new fleet and no update to fleet-other", got, fleet.updateCalls)
	}
}

func TestFleetUpdatable(t *testing.T) {
	tests := []struct {
		status  gltypes.ContainerFleetStatus
		wantErr string
	}{
		{status: gltypes.ContainerFleetStatusActive},
		{status: gltypes.ContainerFleetStatusUpdating, wantErr: "wait for it to become ACTIVE"},
		{status: gltypes.ContainerFleetStatusActivating, wantErr: "wait for it to become ACTIVE"},
		{status: gltypes.ContainerFleetStatusDeleting, wantErr: "being deleted"},
		{status: gltypes.ContainerFleetStatusExpired, wantErr: "deploy destroy"},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			err := fleetUpdatable("fleet-1", tt.status)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("fleetUpdatable() = %v, want nil", err)
				}
				return
			}
			assertErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSelectFleetForDeploy(t *testing.T) {
	owned := fleetCandidate{fleetID: "fleet-mine", ownedName: "mine"}
	foreign := fleetCandidate{fleetID: "fleet-theirs", ownedName: "theirs"}
	untagged := fleetCandidate{fleetID: "fleet-untagged"}
	tests := []struct {
		name       string
		recorded   string
		candidates []fleetCandidate
		wantID     string
		wantErr    string
	}{
		{name: "no candidates", wantID: ""},
		{name: "recorded fleet wins", recorded: "fleet-untagged", candidates: []fleetCandidate{foreign, untagged}, wantID: "fleet-untagged"},
		{name: "tagged for this deployment", candidates: []fleetCandidate{foreign, owned}, wantID: "fleet-mine"},
		{name: "only another deployment's fleet is never selected", candidates: []fleetCandidate{foreign}, wantID: ""},
		{name: "an untagged fleet is never adopted", candidates: []fleetCandidate{untagged}, wantID: ""},
		{name: "stale record falls back to the tag", recorded: "fleet-gone", candidates: []fleetCandidate{owned}, wantID: "fleet-mine"},
		{name: "two fleets tagged for this deployment", candidates: []fleetCandidate{owned, {fleetID: "fleet-mine-2", ownedName: "mine"}}, wantErr: "cannot identify"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectFleetForDeploy(tt.recorded, "mine", tt.candidates)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			gotID := ""
			if got != nil {
				gotID = got.fleetID
			}
			if err != nil || gotID != tt.wantID {
				t.Errorf("selectFleetForDeploy() = %q, %v; want %q", gotID, err, tt.wantID)
			}
		})
	}
}

func TestWarnInstanceTypeDrift(t *testing.T) {
	tests := []struct {
		name     string
		live     string
		wanted   string
		wantWarn bool
	}{
		{name: "same type", live: "c5.large", wanted: "c5.large"},
		{name: "different type", live: "c5.large", wanted: "c7g.large", wantWarn: true},
		{name: "unknown live type", wanted: "c7g.large"},
		{name: "unset option", live: "c5.large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			d := &Deployer{opts: DeployOptions{InstanceType: tt.wanted}, outputWriter: &out}
			d.warnInstanceTypeDrift(&gltypes.ContainerFleet{InstanceType: aws.String(tt.live)})
			if warned := strings.Contains(out.String(), "cannot be changed in place"); warned != tt.wantWarn {
				t.Errorf("warning printed = %v, want %v (output %q)", warned, tt.wantWarn, out.String())
			}
		})
	}
}

func TestDeployFleetErrorPaths(t *testing.T) {
	apiErr := &cgdAPIError{code: "InternalServiceException"}
	tests := []struct {
		name    string
		mutate  func(*updateScenario)
		wantErr string
	}{
		{name: "invalid options", mutate: func(s *updateScenario) { s.d.opts.IAMPath = "/team/" }, wantErr: "path"},
		{name: "listing fleets fails", mutate: func(s *updateScenario) { s.fleet.listOut, s.fleet.listErr = nil, apiErr }, wantErr: "listing fleets"},
		{name: "describing the fleet fails", mutate: func(s *updateScenario) { s.fleet.describeQueue, s.fleet.describeErr = nil, apiErr }, wantErr: "describing fleet fleet-1"},
		{name: "describing the fleet's definition fails", mutate: func(s *updateScenario) { s.cgd.describeResults[0] = cgdDescribeResult{err: apiErr} }, wantErr: "describing container group definition"},
		{name: "definition response is empty", mutate: func(s *updateScenario) {
			s.cgd.describeResults[0] = cgdDescribeResult{out: &gamelift.DescribeContainerGroupDefinitionOutput{}}
		}, wantErr: "empty response"},
		{name: "creating the version fails", mutate: func(s *updateScenario) { s.cgd.updateResults[0] = cgdUpdateResult{err: apiErr} }, wantErr: "creating container group definition version"},
		{name: "new version has no ARN", mutate: func(s *updateScenario) {
			s.cgd.updateResults[0] = cgdUpdateResult{out: &gamelift.UpdateContainerGroupDefinitionOutput{}}
		}, wantErr: "missing definition ARN"},
		{name: "updating the fleet fails", mutate: func(s *updateScenario) { s.fleet.updateErr = apiErr }, wantErr: "updating container fleet"},
		{name: "polling the deployment fails", mutate: func(s *updateScenario) { s.fleet.deploymentOut, s.fleet.deployErr = nil, apiErr }, wantErr: "polling fleet deployment dep-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := chooseFixture()
			running, next := scenarioDefs(probe, "sha256:old", "sha256:new")
			s := newUpdateScenario(t, running, running, next, "sha256:new")
			tt.mutate(s)

			got, err := s.d.DeployFleet(context.Background(), false)

			assertErrorContains(t, err, tt.wantErr)
			if got != nil {
				t.Errorf("DeployFleet() = %+v, want nil result on error", got)
			}
		})
	}
}

// When ECR cannot be read, the deploy still compares URIs, says the digest
// went unchecked, and does not silently roll or silently claim certainty.
func TestDeployFleetSaysWhenTheDigestCannotBeChecked(t *testing.T) {
	probe := chooseFixture()
	running, _ := scenarioDefs(probe, "sha256:same", "sha256:same")
	s := newUpdateScenario(t, running, running, running, "")
	s.ecr.out, s.ecr.err = nil, &cgdAPIError{code: "AccessDeniedException"}
	var out strings.Builder
	s.d.outputWriter = &out

	got, err := s.d.DeployFleet(context.Background(), false)
	if err != nil {
		t.Fatalf("DeployFleet() error = %v", err)
	}

	if got.Action != ActionUnchanged {
		t.Errorf("Action = %q, want unchanged", got.Action)
	}
	for _, want := range []string{"cannot read the image digest", "pass --force"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	assertNoMutations(t, s)
}

func TestDeploymentLabel(t *testing.T) {
	if got := deploymentLabel(fleetDeploymentSnapshot{}); got != "not started" {
		t.Errorf("deploymentLabel(empty) = %q, want not started", got)
	}
	if got := deploymentLabel(fleetDeploymentSnapshot{deployStatus: gltypes.DeploymentStatusInProgress}); got != "IN_PROGRESS" {
		t.Errorf("deploymentLabel(in progress) = %q, want IN_PROGRESS", got)
	}
}
