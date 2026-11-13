package ec2fleet

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/smithy-go"
	"github.com/jpvelasco/ludus/internal/tags"
)

func TestRoleOwnershipGuardsAllEC2Mutations(t *testing.T) {
	for _, key := range []string{"ManagedBy", "Project", "ludus:fleet-name", "ludus:target", "aws:cloudformation:stack-name"} {
		t.Run(key, func(t *testing.T) {
			client := &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:existing")}
			d := &Deployer{iamClient: client, opts: testRoleOptions()}
			observed := tags.Merge(d.opts.resourceTags(), map[string]string{key: "other"})
			client.getRoleOut.Role.Tags = tags.ToIAMTags(observed)
			_, err := d.ensureIAMRole(context.Background())
			if err == nil || !strings.Contains(err.Error(), "refusing to reuse IAM role") {
				t.Fatalf("expected ownership refusal, got %v", err)
			}
			if err := d.deleteIAMRole(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertNoRoleMutation(t, client)
		})
	}
}

func assertNoRoleMutation(t *testing.T, client *fakeIAMClient) {
	t.Helper()
	for name, calls := range map[string]int{
		"create": client.createRoleCalls, "attach": client.attachCalls, "detach": client.detachCalls,
		"delete": client.deleteCalls, "put inline": client.putRolePolicyCnt,
		"delete inline": client.deletePolicyCalls, "list policies": client.listAttachCalls,
	} {
		if calls != 0 {
			t.Errorf("%s called %d times after refusal", name, calls)
		}
	}
}

func TestEC2CustomRoleIdentity(t *testing.T) {
	client := &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:existing")}
	d := &Deployer{iamClient: client, opts: testRoleOptions()}
	d.opts.Tags = map[string]string{"managedby": "example-team", "project": "example-game"}
	client.getRoleOut.Role.Tags = tags.ToIAMTags(d.opts.resourceTags())
	if _, err := d.ensureIAMRole(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.deleteIAMRole(context.Background()); err != nil {
		t.Fatal(err)
	}
	for name, calls := range map[string]int{
		"attach": client.attachCalls, "put inline": client.putRolePolicyCnt,
		"detach": client.detachCalls, "delete inline": client.deletePolicyCalls, "delete": client.deleteCalls,
	} {
		if calls != 1 {
			t.Errorf("%s called %d times, want 1", name, calls)
		}
	}
}

func TestEC2RoleInspectionFailures(t *testing.T) {
	tests := []struct {
		name   string
		client *fakeIAMClient
	}{
		{"access denied", &fakeIAMClient{getRoleErr: &smithy.GenericAPIError{Code: "AccessDenied"}}},
		{"nil output", &fakeIAMClient{}},
		{"nil role", &fakeIAMClient{getRoleOut: &iam.GetRoleOutput{}}},
		{"unrecognized absence", &fakeIAMClient{getRoleErr: &smithy.GenericAPIError{Code: "NotFoundException"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Deployer{iamClient: tt.client, opts: testRoleOptions()}
			_, ensureErr := d.ensureIAMRole(context.Background())
			deleteErr := d.deleteIAMRole(context.Background())
			for _, err := range []error{ensureErr, deleteErr} {
				if err == nil {
					t.Error("inspection failure was ignored")
				}
			}
			assertNoRoleMutation(t, tt.client)
		})
	}
}

func TestEC2RoleCreationTags(t *testing.T) {
	original := iamPropagationWait
	iamPropagationWait = 0
	t.Cleanup(func() { iamPropagationWait = original })
	client := &fakeIAMClient{getRoleErr: notFoundPolicyErr()}
	d := &Deployer{iamClient: client, opts: testRoleOptions()}
	d.opts.Tags = map[string]string{"managedby": "example-team", "project": "example-game"}
	if _, err := d.ensureIAMRole(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string)
	for _, tag := range client.createRoleInput.Tags {
		got[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	if !reflect.DeepEqual(got, d.opts.resourceTags()) {
		t.Errorf("created role tags = %v, want %v", got, d.opts.resourceTags())
	}
}

func TestEC2RoleCreationFailure(t *testing.T) {
	for _, tt := range []struct {
		name   string
		client *fakeIAMClient
	}{
		{"malformed role", &fakeIAMClient{createRoleOut: &iam.CreateRoleOutput{}}},
		{"create race", &fakeIAMClient{createRoleErr: &smithy.GenericAPIError{Code: "EntityAlreadyExists"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.client.getRoleErr = notFoundPolicyErr()
			d := &Deployer{iamClient: tt.client, opts: testRoleOptions()}
			if _, err := d.ensureIAMRole(context.Background()); err == nil {
				t.Fatal("creation failure ignored")
			}
			if tt.client.attachCalls != 0 || tt.client.putRolePolicyCnt != 0 {
				t.Fatal("policy mutation after failed creation")
			}
		})
	}
}
