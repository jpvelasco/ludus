package gamelift

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

const testECRHost = "123456789012.dkr.ecr.us-west-2.amazonaws.com"

type fakeECRClient struct {
	out    *ecr.DescribeImagesOutput
	err    error
	calls  int
	inputs []*ecr.DescribeImagesInput
}

func (f *fakeECRClient) DescribeImages(_ context.Context, in *ecr.DescribeImagesInput, _ ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error) {
	f.calls++
	f.inputs = append(f.inputs, in)
	return f.out, f.err
}

func ecrDigestOut(digest string) *ecr.DescribeImagesOutput {
	return &ecr.DescribeImagesOutput{ImageDetails: []ecrtypes.ImageDetail{{ImageDigest: aws.String(digest)}}}
}

func TestParseImageURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    imageRef
		wantErr string
	}{
		{name: "tagged", uri: testECRHost + "/ludus-server:v2", want: imageRef{registryID: "123456789012", repository: "ludus-server", tag: "v2"}},
		{name: "untagged defaults to latest", uri: testECRHost + "/ludus-server", want: imageRef{registryID: "123456789012", repository: "ludus-server", tag: "latest"}},
		{name: "nested repository", uri: testECRHost + "/team/ludus-server:1", want: imageRef{registryID: "123456789012", repository: "team/ludus-server", tag: "1"}},
		{name: "digest pinned", uri: testECRHost + "/ludus-server@sha256:abc", want: imageRef{registryID: "123456789012", repository: "ludus-server", digest: "sha256:abc"}},
		{name: "not an ECR host", uri: "docker.io/library/nginx:latest", wantErr: "not an ECR registry"},
		{name: "no repository", uri: testECRHost, wantErr: "not registry/repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseImageURI(tt.uri)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("parseImageURI(%q) = %+v, %v; want %+v", tt.uri, got, err, tt.want)
			}
		})
	}
}

func defWithDigest(digest string) *gltypes.ContainerGroupDefinition {
	return &gltypes.ContainerGroupDefinition{GameServerContainerDefinition: &gltypes.GameServerContainerDefinition{
		ResolvedImageDigest: aws.String(digest),
	}}
}

func TestCompareDigest(t *testing.T) {
	tests := []struct {
		name string
		def  *gltypes.ContainerGroupDefinition
		ecr  string
		want digestComparison
	}{
		{name: "nil definition", ecr: "sha256:a", want: digestUnknown},
		{name: "no game server container", def: &gltypes.ContainerGroupDefinition{}, ecr: "sha256:a", want: digestUnknown},
		{name: "ECR digest unknown", def: defWithDigest("sha256:a"), want: digestUnknown},
		{name: "snapshot digest missing", def: defWithDigest(""), ecr: "sha256:a", want: digestUnknown},
		{name: "same", def: defWithDigest("sha256:a"), ecr: "sha256:a", want: digestSame},
		{name: "differs", def: defWithDigest("sha256:a"), ecr: "sha256:b", want: digestDiffers},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareDigest(tt.def, tt.ecr); got != tt.want {
				t.Errorf("compareDigest() = %v, want %v", got, tt.want)
			}
		})
	}
}

// chooseFixture builds a deployer whose desired definition the helpers below
// derive described definitions from, so "matching" cannot drift from the
// real desired input.
func chooseFixture() *Deployer {
	return &Deployer{opts: DeployOptions{ImageURI: testECRHost + "/ludus-server:latest", ServerPort: 7777, ContainerGroupName: "test-group"}}
}

// describedDefinition renders the deployer's desired input as the definition
// GameLift would describe, with the given ARN, version, and snapshot digest.
func describedDefinition(d *Deployer, arn string, version int32, digest string) *gltypes.ContainerGroupDefinition {
	in := d.containerGroupDefinitionInput()
	gs := in.GameServerContainerDefinition
	return &gltypes.ContainerGroupDefinition{
		ContainerGroupDefinitionArn: aws.String(arn),
		VersionNumber:               aws.Int32(version),
		Status:                      gltypes.ContainerGroupDefinitionStatusReady,
		TotalMemoryLimitMebibytes:   in.TotalMemoryLimitMebibytes,
		TotalVcpuLimit:              in.TotalVcpuLimit,
		GameServerContainerDefinition: &gltypes.GameServerContainerDefinition{
			ImageUri:            gs.ImageUri,
			ServerSdkVersion:    gs.ServerSdkVersion,
			PortConfiguration:   gs.PortConfiguration,
			ResolvedImageDigest: aws.String(digest),
		},
	}
}

// staleDefinition is describedDefinition running a different image.
func staleDefinition(d *Deployer, arn string, version int32, digest string) *gltypes.ContainerGroupDefinition {
	def := describedDefinition(d, arn, version, digest)
	def.GameServerContainerDefinition.ImageUri = aws.String(testECRHost + "/ludus-server:old")
	return def
}

func TestChooseUpdate(t *testing.T) {
	d := chooseFixture()
	desired := d.containerGroupDefinitionInput()
	tests := []struct {
		name           string
		running        *gltypes.ContainerGroupDefinition
		latest         *gltypes.ContainerGroupDefinition
		digest         string
		force          bool
		want           updateAction
		wantUnverified bool
		wantReason     string
	}{
		{name: "force always cuts a new version", running: describedDefinition(d, "v1", 1, "sha256:a"), latest: describedDefinition(d, "v1", 1, "sha256:a"), digest: "sha256:a", force: true, want: updateNewVersion, wantReason: "--force"},
		{name: "same image and digest is unchanged", running: describedDefinition(d, "v1", 1, "sha256:a"), latest: describedDefinition(d, "v1", 1, "sha256:a"), digest: "sha256:a", want: updateNone},
		{name: "unreadable digest is unchanged but unverified", running: describedDefinition(d, "v1", 1, "sha256:a"), latest: describedDefinition(d, "v1", 1, "sha256:a"), want: updateNone, wantUnverified: true},
		{name: "re-pushed tag cuts a new version", running: describedDefinition(d, "v1", 1, "sha256:a"), latest: describedDefinition(d, "v1", 1, "sha256:a"), digest: "sha256:b", want: updateNewVersion, wantReason: "re-pushed"},
		{name: "new image cuts a new version", running: staleDefinition(d, "v1", 1, "sha256:a"), latest: staleDefinition(d, "v1", 1, "sha256:a"), digest: "sha256:a", want: updateNewVersion, wantReason: "definition changed"},
		{name: "fleet behind a matching latest only rolls", running: staleDefinition(d, "v1", 1, "sha256:a"), latest: describedDefinition(d, "v2", 2, "sha256:a"), digest: "sha256:a", want: updateRollOnly},
		{name: "reverting after a failed rollout needs no new version", running: describedDefinition(d, "v1", 1, "sha256:a"), latest: staleDefinition(d, "v2", 2, "sha256:z"), digest: "sha256:a", want: updateNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chooseUpdate(tt.running, tt.latest, desired, tt.digest, tt.force)
			if got.action != tt.want || got.unverified != tt.wantUnverified {
				t.Errorf("chooseUpdate() = %+v, want action %v unverified %v", got, tt.want, tt.wantUnverified)
			}
			if !strings.Contains(got.reason, tt.wantReason) {
				t.Errorf("chooseUpdate() reason = %q, want it to contain %q", got.reason, tt.wantReason)
			}
		})
	}
}

func TestDesiredImageDigest(t *testing.T) {
	tests := []struct {
		name      string
		uri       string
		client    *fakeECRClient
		want      string
		wantErr   string
		wantCalls int
	}{
		{name: "tag resolved through ECR", uri: testECRHost + "/ludus-server:latest", client: &fakeECRClient{out: ecrDigestOut("sha256:a")}, want: "sha256:a", wantCalls: 1},
		{name: "pinned digest needs no lookup", uri: testECRHost + "/ludus-server@sha256:b", client: &fakeECRClient{}, want: "sha256:b"},
		{name: "ECR error is wrapped", uri: testECRHost + "/ludus-server:latest", client: &fakeECRClient{err: errors.New("AccessDeniedException")}, wantErr: "reading the digest", wantCalls: 1},
		{name: "missing tag", uri: testECRHost + "/ludus-server:latest", client: &fakeECRClient{out: &ecr.DescribeImagesOutput{}}, wantErr: "no image tagged", wantCalls: 1},
		{name: "non-ECR host", uri: "docker.io/x/y:1", client: &fakeECRClient{}, wantErr: "not an ECR registry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Deployer{opts: DeployOptions{ImageURI: tt.uri}, ecrClient: tt.client}
			got, err := d.desiredImageDigest(context.Background())
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
			} else if err != nil || got != tt.want {
				t.Errorf("desiredImageDigest() = %q, %v; want %q", got, err, tt.want)
			}
			if tt.client.calls != tt.wantCalls {
				t.Errorf("DescribeImages calls = %d, want %d", tt.client.calls, tt.wantCalls)
			}
		})
	}
}

func TestDesiredImageDigestSendsRegistryAndTag(t *testing.T) {
	client := &fakeECRClient{out: ecrDigestOut("sha256:a")}
	d := &Deployer{opts: DeployOptions{ImageURI: testECRHost + "/team/ludus-server:v7"}, ecrClient: client}
	if _, err := d.desiredImageDigest(context.Background()); err != nil {
		t.Fatalf("desiredImageDigest() error = %v", err)
	}
	in := client.inputs[0]
	if aws.ToString(in.RegistryId) != "123456789012" || aws.ToString(in.RepositoryName) != "team/ludus-server" ||
		aws.ToString(in.ImageIds[0].ImageTag) != "v7" {
		t.Errorf("DescribeImages input = %+v, want registry 123456789012, repo team/ludus-server, tag v7", in)
	}
}

func TestDesiredImageDigestWithoutClient(t *testing.T) {
	d := &Deployer{opts: DeployOptions{ImageURI: testECRHost + "/ludus-server:latest"}}
	_, err := d.desiredImageDigest(context.Background())
	assertErrorContains(t, err, "no ECR client")
}
