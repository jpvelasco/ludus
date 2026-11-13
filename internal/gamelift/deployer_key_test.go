package gamelift

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/jpvelasco/ludus/internal/tags"
)

func TestFleetDiscoveryCustomKeyAndLegacyFallback(t *testing.T) {
	for _, key := range []string{"Deployment", fleetNameTagKey} {
		t.Run(key, func(t *testing.T) {
			client := &fakeFleetClient{tagsOut: &gamelift.ListTagsForResourceOutput{
				Tags: []gltypes.Tag{{Key: aws.String(key), Value: aws.String("test-fleet")}},
			}}
			d := newTestFleetDeployer(client, &fakeIAMClient{})
			d.opts.OwnershipTagKey = "Deployment"
			fleet := gltypes.ContainerFleet{FleetArn: aws.String("arn:example")}
			if got := d.fleetNameTag(context.Background(), fleet); got != "test-fleet" {
				t.Fatalf("fleet tag = %q", got)
			}
		})
	}
}

func TestFleetDiscoveryRefusesConflictingDuplicateTags(t *testing.T) {
	client := &fakeFleetClient{tagsOut: &gamelift.ListTagsForResourceOutput{
		Tags: []gltypes.Tag{
			{Key: aws.String("Deployment"), Value: aws.String("first")},
			{Key: aws.String("Deployment"), Value: aws.String("second")},
		},
	}}
	d := newTestFleetDeployer(client, &fakeIAMClient{})
	d.opts.OwnershipTagKey = "Deployment"
	if got := d.fleetNameTag(context.Background(), gltypes.ContainerFleet{FleetArn: aws.String("arn:example")}); got != "" {
		t.Fatalf("ambiguous attribution = %q", got)
	}
}

func TestRoleCustomKeyReuseAndDeletion(t *testing.T) {
	client := &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:example")}
	d := newTestIAMDeployer(client)
	d.opts.OwnershipTagKey = "Deployment"
	client.getRoleOut.Role.Tags = tags.ToIAMTags(d.resourceTags())
	if _, legacy := d.resourceTags()[fleetNameTagKey]; legacy {
		t.Fatal("default key was written with a custom key")
	}
	if _, err := d.ensureIAMRole(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.deleteIAMRole(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.getRoleOut.Role.Tags = tags.ToIAMTags(map[string]string{
		"ManagedBy": "ludus", "Project": "test-project", "Deployment": "other",
	})
	client = &fakeIAMClient{getRoleOut: client.getRoleOut}
	d.iamClient = client
	assertIAMOwnershipRefusal(t, d, client)
}

func TestInvalidOwnershipKeyStopsGroupCreation(t *testing.T) {
	d := &Deployer{opts: DeployOptions{OwnershipTagKey: "Project"}}
	if _, err := d.CreateContainerGroupDefinition(context.Background()); err == nil {
		t.Fatal("reserved key reached AWS")
	}
}

func TestRoleCreationWritesConfiguredKey(t *testing.T) {
	client := &fakeIAMClient{
		getRoleErr:    &cgdAPIError{code: "NoSuchEntity"},
		createRoleOut: &iam.CreateRoleOutput{Role: iamRoleOutput("arn:created").Role},
	}
	d := newTestIAMDeployer(client)
	d.opts.OwnershipTagKey = "Deployment"
	if _, err := d.ensureIAMRole(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string)
	for _, tag := range client.createRoleInput.Tags {
		got[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	if got["Deployment"] != "test-fleet" || got[fleetNameTagKey] != "" {
		t.Fatalf("creation tags = %v", got)
	}
}
