package gamelift

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/jpvelasco/ludus/internal/tags"
)

func TestIAMRoleRejectsForeignOwnership(t *testing.T) {
	tests := []struct{ name, key, value string }{
		{"different project", "Project", "other"},
		{"different fleet", "ludus:fleet-name", "other"},
		{"different manager", "ManagedBy", "other"},
		{"legacy missing attribution", "Project", ""},
		{"CloudFormation role", "aws:cloudformation:stack-name", "example"},
		{"ambiguous project", "project", "other"},
		{"EC2 role", "ludus:target", "ec2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:existing")}
			d := newTestIAMDeployer(client)
			observed := tags.Merge(d.resourceTags(), map[string]string{tt.key: tt.value})
			client.getRoleOut.Role.Tags = tags.ToIAMTags(observed)
			// Preserve variants rather than letting the creation converter deduplicate them.
			if tt.key == "project" {
				client.getRoleOut.Role.Tags = append(client.getRoleOut.Role.Tags, iamtypes.Tag{Key: aws.String("project"), Value: aws.String("other")})
			}
			assertIAMOwnershipRefusal(t, d, client)
		})
	}
}

func assertIAMOwnershipRefusal(t *testing.T, d *Deployer, client *fakeIAMClient) {
	t.Helper()
	var output bytes.Buffer
	d.SetOutput(&output)
	d.opts.IAMRoleName = "explicit-name"
	_, err := d.ensureIAMRole(context.Background())
	if err == nil || !strings.Contains(err.Error(), "gamelift.iam.roleName") {
		t.Fatalf("ensureIAMRole() = %v; want ownership refusal and remedy", err)
	}
	if err := d.deleteIAMRole(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Skipping IAM role explicit-name") {
		t.Fatalf("missing skip reason: %s", output.String())
	}
	assertNoIAMMutations(t, client)
}

func assertNoIAMMutations(t *testing.T, client *fakeIAMClient) {
	t.Helper()
	for name, calls := range map[string]int{
		"create": client.createRoleCalls, "attach": client.attachCalls,
		"detach": client.detachCalls, "delete": client.deleteRoleCalls,
		"list policies": client.listAttachCalls,
	} {
		if calls != 0 {
			t.Errorf("%s called %d times after refusal", name, calls)
		}
	}
}

func TestIAMRoleInspectionFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		client *fakeIAMClient
	}{
		{"access denied", &fakeIAMClient{getRoleErr: &cgdAPIError{code: "AccessDeniedException"}}},
		{"transient error", &fakeIAMClient{getRoleErr: &cgdAPIError{code: "ServiceFailure"}}},
		{"unrecognized not found", &fakeIAMClient{getRoleErr: &cgdAPIError{code: "NotFoundException"}}},
		{"nil output", &fakeIAMClient{}},
		{"nil role", &fakeIAMClient{getRoleOut: &iam.GetRoleOutput{}}},
		{"empty ARN", &fakeIAMClient{getRoleOut: &iam.GetRoleOutput{Role: &iamtypes.Role{}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestIAMDeployer(tt.client)
			_, ensureErr := d.ensureIAMRole(context.Background())
			deleteErr := d.deleteIAMRole(context.Background())
			for _, err := range []error{ensureErr, deleteErr} {
				if err == nil || !strings.Contains(err.Error(), "inspecting IAM role") {
					t.Errorf("expected inspection failure, got %v", err)
				}
			}
			assertNoIAMMutations(t, tt.client)
		})
	}
}

func TestIAMRoleCustomIdentity(t *testing.T) {
	client := &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:existing")}
	d := newTestIAMDeployer(client)
	d.opts.Tags = map[string]string{"managedby": "example-team", "project": "example-game"}
	client.getRoleOut.Role.Tags = tags.ToIAMTags(d.resourceTags())
	if _, err := d.ensureIAMRole(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.deleteIAMRole(context.Background()); err != nil {
		t.Fatal(err)
	}
	for name, calls := range map[string]int{"attach": client.attachCalls, "detach": client.detachCalls, "delete": client.deleteRoleCalls} {
		if calls != 1 {
			t.Errorf("%s calls = %d, want 1", name, calls)
		}
	}
}

func TestIAMRoleInvalidExpectedIdentity(t *testing.T) {
	client := &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:existing")}
	d := newTestIAMDeployer(client)
	d.opts.Tags = nil
	_, ensureErr := d.ensureIAMRole(context.Background())
	deleteErr := d.deleteIAMRole(context.Background())
	for _, err := range []error{ensureErr, deleteErr} {
		if err == nil || !strings.Contains(err.Error(), "validating IAM role ownership") {
			t.Errorf("expected invalid identity error, got %v", err)
		}
	}
	if client.getRoleCalls != 0 {
		t.Fatal("invalid identity made an AWS call")
	}
	assertNoIAMMutations(t, client)
}

func TestIAMRoleMissingDeletion(t *testing.T) {
	client := &fakeIAMClient{getRoleErr: &cgdAPIError{code: "NoSuchEntity"}}
	if err := newTestIAMDeployer(client).deleteIAMRole(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertNoIAMMutations(t, client)
}

func TestIAMRoleMalformedCreation(t *testing.T) {
	client := &fakeIAMClient{getRoleErr: &cgdAPIError{code: "NoSuchEntity"}}
	_, err := newTestIAMDeployer(client).ensureIAMRole(context.Background())
	if err == nil || !strings.Contains(err.Error(), "missing role ARN") {
		t.Fatalf("expected malformed creation error, got %v", err)
	}
	if client.attachCalls != 0 {
		t.Fatal("attached policy after malformed creation")
	}
}
