package gamelift

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

// fakeGameLift is a small stateful model of the GameLift and ECR behavior the
// in-place update depends on. It is not a general GameLift emulator: it keeps
// only what is needed to run DeployFleet end to end across several deploys.
//
// The behavior it encodes was checked against the real services with a probe
// (container group definitions only): ECR's digest for a tag equals the
// definition's ResolvedImageDigest, a re-pushed tag leaves the latest version's
// snapshot stale, UpdateContainerGroupDefinition re-snapshots the tag, a
// version bump with nothing changed is accepted, versioned ARNs address one
// version, and describing by name returns the latest version.
//
// Fleet-side behavior is the model's assumption, not an observation: a fleet
// created from a definition name records that version's ARN, a rolling update
// that fails validation leaves the fleet on its previous version, and updating
// a fleet to the version it already runs starts no deployment.
type fakeGameLift struct {
	d        *Deployer
	tags     map[string]string // ECR: image tag -> digest
	healthy  map[string]bool   // image digest -> its containers pass fleet validation
	versions []*gltypes.ContainerGroupDefinition
	fleet    *gltypes.ContainerFleet
	deploys  map[string]gltypes.DeploymentStatus

	fleetCreates int
	fleetUpdates int
}

const lifecycleFleetID = "fleet-1"

func versionARN(version int) string {
	return fmt.Sprintf("arn:aws:gamelift:us-west-2:123456789012:containergroupdefinition/test-group:%d", version)
}

func (f *fakeGameLift) digestOf(imageURI *string) string {
	ref, err := parseImageURI(aws.ToString(imageURI))
	if err != nil {
		return ""
	}
	return f.tags[ref.tag]
}

func (f *fakeGameLift) latest() *gltypes.ContainerGroupDefinition {
	if len(f.versions) == 0 {
		return nil
	}
	return f.versions[len(f.versions)-1]
}

// resolve finds a definition by group name (the latest version) or by
// version-specific ARN.
func (f *fakeGameLift) resolve(ref string) *gltypes.ContainerGroupDefinition {
	i := strings.LastIndex(ref, ":")
	if !strings.HasPrefix(ref, "arn:") || i < 0 {
		return f.latest()
	}
	n, err := strconv.Atoi(ref[i+1:])
	if err != nil || n < 1 || n > len(f.versions) {
		return nil
	}
	return f.versions[n-1]
}

func (f *fakeGameLift) addVersion(imageURI *string) *gltypes.ContainerGroupDefinition {
	n := len(f.versions) + 1
	def := describedDefinition(f.d, versionARN(n), int32(n), f.digestOf(imageURI))
	f.versions = append(f.versions, def)
	return def
}

// fleetVersion is the definition version the fleet currently runs.
func (f *fakeGameLift) fleetVersion() int32 {
	def := f.resolve(aws.ToString(f.fleet.GameServerContainerGroupDefinitionArn))
	if def == nil {
		return 0
	}
	return aws.ToInt32(def.VersionNumber)
}

func (f *fakeGameLift) DescribeImages(_ context.Context, in *ecr.DescribeImagesInput, _ ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error) {
	digest, ok := f.tags[aws.ToString(in.ImageIds[0].ImageTag)]
	if !ok {
		return &ecr.DescribeImagesOutput{}, nil
	}
	return ecrDigestOut(digest), nil
}

func (f *fakeGameLift) CreateContainerGroupDefinition(_ context.Context, in *gamelift.CreateContainerGroupDefinitionInput, _ ...func(*gamelift.Options)) (*gamelift.CreateContainerGroupDefinitionOutput, error) {
	if len(f.versions) > 0 {
		return nil, &cgdAPIError{code: "ConflictException"}
	}
	def := f.addVersion(in.GameServerContainerDefinition.ImageUri)
	return &gamelift.CreateContainerGroupDefinitionOutput{ContainerGroupDefinition: def}, nil
}

func (f *fakeGameLift) UpdateContainerGroupDefinition(_ context.Context, in *gamelift.UpdateContainerGroupDefinitionInput, _ ...func(*gamelift.Options)) (*gamelift.UpdateContainerGroupDefinitionOutput, error) {
	latest := f.latest()
	if latest == nil || aws.ToInt32(in.SourceVersionNumber) != aws.ToInt32(latest.VersionNumber) {
		return nil, &cgdAPIError{code: "InvalidRequestException"}
	}
	def := f.addVersion(in.GameServerContainerDefinition.ImageUri)
	return &gamelift.UpdateContainerGroupDefinitionOutput{ContainerGroupDefinition: def}, nil
}

func (f *fakeGameLift) DescribeContainerGroupDefinition(_ context.Context, in *gamelift.DescribeContainerGroupDefinitionInput, _ ...func(*gamelift.Options)) (*gamelift.DescribeContainerGroupDefinitionOutput, error) {
	def := f.resolve(aws.ToString(in.Name))
	if def == nil {
		return nil, &cgdAPIError{code: "NotFoundException"}
	}
	return &gamelift.DescribeContainerGroupDefinitionOutput{ContainerGroupDefinition: def}, nil
}

func (f *fakeGameLift) DeleteContainerGroupDefinition(context.Context, *gamelift.DeleteContainerGroupDefinitionInput, ...func(*gamelift.Options)) (*gamelift.DeleteContainerGroupDefinitionOutput, error) {
	f.versions = nil
	return &gamelift.DeleteContainerGroupDefinitionOutput{}, nil
}

func (f *fakeGameLift) CreateContainerFleet(_ context.Context, in *gamelift.CreateContainerFleetInput, _ ...func(*gamelift.Options)) (*gamelift.CreateContainerFleetOutput, error) {
	def := f.resolve(aws.ToString(in.GameServerContainerGroupDefinitionName))
	if def == nil {
		return nil, &cgdAPIError{code: "NotFoundException"}
	}
	f.fleetCreates++
	f.deploys = map[string]gltypes.DeploymentStatus{"dep-0": gltypes.DeploymentStatusComplete}
	f.fleet = &gltypes.ContainerFleet{
		FleetId:                               aws.String(lifecycleFleetID),
		FleetArn:                              aws.String("arn:aws:gamelift:us-west-2:123456789012:containerfleet/" + lifecycleFleetID),
		Status:                                gltypes.ContainerFleetStatusActive,
		InstanceType:                          aws.String("c5.large"),
		GameServerContainerGroupDefinitionArn: def.ContainerGroupDefinitionArn,
		DeploymentDetails:                     &gltypes.DeploymentDetails{LatestDeploymentId: aws.String("dep-0")},
	}
	return &gamelift.CreateContainerFleetOutput{ContainerFleet: f.fleet}, nil
}

func (f *fakeGameLift) UpdateContainerFleet(_ context.Context, in *gamelift.UpdateContainerFleetInput, _ ...func(*gamelift.Options)) (*gamelift.UpdateContainerFleetOutput, error) {
	f.fleetUpdates++
	target := f.resolve(aws.ToString(in.GameServerContainerGroupDefinitionName))
	if f.fleet == nil || target == nil {
		return nil, &cgdAPIError{code: "NotFoundException"}
	}
	targetARN := aws.ToString(target.ContainerGroupDefinitionArn)
	if targetARN == aws.ToString(f.fleet.GameServerContainerGroupDefinitionArn) {
		return &gamelift.UpdateContainerFleetOutput{}, nil // nothing to change: no deployment starts
	}
	id := fmt.Sprintf("dep-%d", len(f.deploys))
	f.fleet.DeploymentDetails = &gltypes.DeploymentDetails{LatestDeploymentId: aws.String(id)}
	if f.healthy[aws.ToString(target.GameServerContainerDefinition.ResolvedImageDigest)] {
		f.deploys[id] = gltypes.DeploymentStatusComplete
		f.fleet.GameServerContainerGroupDefinitionArn = target.ContainerGroupDefinitionArn
	} else {
		f.deploys[id] = gltypes.DeploymentStatusRollbackInProgress // the fleet keeps its previous version
	}
	return &gamelift.UpdateContainerFleetOutput{}, nil
}

func (f *fakeGameLift) DescribeContainerFleet(context.Context, *gamelift.DescribeContainerFleetInput, ...func(*gamelift.Options)) (*gamelift.DescribeContainerFleetOutput, error) {
	if f.fleet == nil {
		return nil, &cgdAPIError{code: "NotFoundException"}
	}
	snapshot := *f.fleet
	return &gamelift.DescribeContainerFleetOutput{ContainerFleet: &snapshot}, nil
}

func (f *fakeGameLift) ListContainerFleets(context.Context, *gamelift.ListContainerFleetsInput, ...func(*gamelift.Options)) (*gamelift.ListContainerFleetsOutput, error) {
	if f.fleet == nil {
		return &gamelift.ListContainerFleetsOutput{}, nil
	}
	return &gamelift.ListContainerFleetsOutput{ContainerFleets: []gltypes.ContainerFleet{*f.fleet}}, nil
}

func (f *fakeGameLift) ListTagsForResource(context.Context, *gamelift.ListTagsForResourceInput, ...func(*gamelift.Options)) (*gamelift.ListTagsForResourceOutput, error) {
	return fleetTagsOut("test-fleet"), nil
}

func (f *fakeGameLift) DescribeFleetDeployment(_ context.Context, in *gamelift.DescribeFleetDeploymentInput, _ ...func(*gamelift.Options)) (*gamelift.DescribeFleetDeploymentOutput, error) {
	return deploymentOutput(f.deploys[aws.ToString(in.DeploymentId)]), nil
}

func (f *fakeGameLift) DeleteContainerFleet(context.Context, *gamelift.DeleteContainerFleetInput, ...func(*gamelift.Options)) (*gamelift.DeleteContainerFleetOutput, error) {
	f.fleet = nil
	return &gamelift.DeleteContainerFleetOutput{}, nil
}

func (f *fakeGameLift) CreateGameSession(context.Context, *gamelift.CreateGameSessionInput, ...func(*gamelift.Options)) (*gamelift.CreateGameSessionOutput, error) {
	return &gamelift.CreateGameSessionOutput{}, nil
}

func (f *fakeGameLift) DescribeGameSessions(context.Context, *gamelift.DescribeGameSessionsInput, ...func(*gamelift.Options)) (*gamelift.DescribeGameSessionsOutput, error) {
	return &gamelift.DescribeGameSessionsOutput{}, nil
}

// push puts an image with the given digest behind the :latest tag, the way
// "rebuild, re-push latest" does. broken images never pass fleet validation.
func (f *fakeGameLift) push(digest string, broken bool) {
	f.tags["latest"] = digest
	f.healthy[digest] = !broken
}

func newLifecycleDeployer(t *testing.T) (*Deployer, *fakeGameLift, *bytes.Buffer) {
	t.Helper()
	t.Chdir(t.TempDir())
	gl := &fakeGameLift{tags: map[string]string{}, healthy: map[string]bool{}}
	out := &bytes.Buffer{}
	d := newTestFleetDeployer(&fakeFleetClient{}, &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:role")})
	d.opts.ImageURI = testECRHost + "/ludus-server:latest"
	d.glClient, d.cgdClient, d.ecrClient = gl, gl, gl
	d.SetOutput(out)
	gl.d = d
	return d, gl, out
}

// lifecycleStep is one `ludus deploy fleet` run in the sequence, with what the
// account should look like afterwards. The want counters are cumulative.
type lifecycleStep struct {
	name   string
	push   string // digest pushed over :latest first; "" pushes nothing
	broken bool   // the pushed image never passes fleet validation
	force  bool

	wantAction         DeployAction
	wantErr            string
	wantVersions       int   // definition versions that exist
	wantRunningVersion int32 // the version the fleet runs
	wantFleetUpdates   int   // UpdateContainerFleet calls so far
	wantOut            string
}

// lifecycleSteps is the dev loop: deploy, redeploy, re-push, force, a bad
// image, a retry, and a revert. Every step must leave exactly one fleet.
func lifecycleSteps() []lifecycleStep {
	return []lifecycleStep{
		{name: "first deploy creates the fleet on image A", push: "sha256:a",
			wantAction: ActionCreated, wantVersions: 1, wantRunningVersion: 1, wantOut: "Creating container fleet"},
		{name: "redeploying unchanged does nothing",
			wantAction: ActionUnchanged, wantVersions: 1, wantRunningVersion: 1, wantOut: "nothing to deploy"},
		{name: "re-pushing :latest as image B rolls the same fleet", push: "sha256:b",
			wantAction: ActionUpdated, wantVersions: 2, wantRunningVersion: 2, wantFleetUpdates: 1, wantOut: "Updating fleet fleet-1 in place"},
		{name: "after the roll the fleet is settled",
			wantAction: ActionUnchanged, wantVersions: 2, wantRunningVersion: 2, wantFleetUpdates: 1, wantOut: "nothing to deploy"},
		{name: "--force cuts a new version although nothing changed", force: true,
			wantAction: ActionUpdated, wantVersions: 3, wantRunningVersion: 3, wantFleetUpdates: 2, wantOut: "--force was given"},
		{name: "a crashing image C rolls back and the fleet keeps serving B", push: "sha256:c", broken: true,
			wantErr: "failed validation", wantVersions: 4, wantRunningVersion: 3, wantFleetUpdates: 3},
		{name: "retrying rolls onto the existing version instead of cutting another",
			wantErr: "failed validation", wantVersions: 4, wantRunningVersion: 3, wantFleetUpdates: 4},
		{name: "reverting the tag to the running image is a no-op", push: "sha256:b",
			wantAction: ActionUnchanged, wantVersions: 4, wantRunningVersion: 3, wantFleetUpdates: 4, wantOut: "nothing to deploy"},
	}
}

func expectEq[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// assertLifecycleOutcome checks what DeployFleet returned: the expected error,
// or the expected action on the one fleet.
func assertLifecycleOutcome(t *testing.T, got *FleetStatus, err error, s lifecycleStep) {
	t.Helper()
	if s.wantErr != "" {
		assertErrorContains(t, err, s.wantErr)
		return
	}
	if err != nil {
		t.Fatalf("DeployFleet() error = %v", err)
	}
	expectEq(t, "action", got.Action, s.wantAction)
	expectEq(t, "fleet ID", got.FleetID, lifecycleFleetID)
}

func assertLifecycleStep(t *testing.T, gl *fakeGameLift, got *FleetStatus, err error, out string, s lifecycleStep) {
	t.Helper()
	assertLifecycleOutcome(t, got, err, s)
	expectEq(t, "fleets ever created", gl.fleetCreates, 1)
	expectEq(t, "definition versions", len(gl.versions), s.wantVersions)
	expectEq(t, "version the fleet runs", gl.fleetVersion(), s.wantRunningVersion)
	expectEq(t, "UpdateContainerFleet calls", gl.fleetUpdates, s.wantFleetUpdates)
	if !strings.Contains(out, s.wantOut) {
		t.Errorf("output does not mention %q:\n%s", s.wantOut, out)
	}
}

// TestDeployFleetLifecycle drives repeated deploys through the real DeployFleet
// and state recording against a stateful GameLift model. It is the automated
// stand-in for the fleet-level end-to-end run: one fleet for the whole
// sequence, an update only when the image really changed, and a bad image never
// displacing the version that serves.
func TestDeployFleetLifecycle(t *testing.T) {
	d, gl, out := newLifecycleDeployer(t)
	for _, s := range lifecycleSteps() {
		ok := t.Run(s.name, func(t *testing.T) {
			if s.push != "" {
				gl.push(s.push, s.broken)
			}
			out.Reset()
			got, err := d.DeployFleet(context.Background(), s.force)
			if err == nil {
				RecordFleetState(got, io.Discard)
			}
			assertLifecycleStep(t, gl, got, err, out.String(), s)
		})
		if !ok {
			break // later steps depend on this one's state
		}
	}
}
