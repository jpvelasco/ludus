package ec2fleet

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"
)

// fakeIAMClient replays canned IAM responses and records calls so the
// ensure/repair/delete guards can be tested hermetically.
type fakeIAMClient struct {
	getRoleOut      *iam.GetRoleOutput
	getRoleErr      error
	listAttachedOut *iam.ListAttachedRolePoliciesOutput

	getRoleCalls int
	attachCalls  int
	detachCalls  int
	deleteCalls  int
}

func (f *fakeIAMClient) GetRole(_ context.Context, _ *iam.GetRoleInput, _ ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	f.getRoleCalls++
	return f.getRoleOut, f.getRoleErr
}

func (f *fakeIAMClient) CreateRole(_ context.Context, _ *iam.CreateRoleInput, _ ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
	return &iam.CreateRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:created")}}, nil
}

func (f *fakeIAMClient) AttachRolePolicy(_ context.Context, in *iam.AttachRolePolicyInput, _ ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
	f.attachCalls++
	return &iam.AttachRolePolicyOutput{}, nil
}

func (f *fakeIAMClient) DetachRolePolicy(_ context.Context, _ *iam.DetachRolePolicyInput, _ ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error) {
	f.detachCalls++
	return &iam.DetachRolePolicyOutput{}, nil
}

func (f *fakeIAMClient) DeleteRole(_ context.Context, _ *iam.DeleteRoleInput, _ ...func(*iam.Options)) (*iam.DeleteRoleOutput, error) {
	f.deleteCalls++
	return &iam.DeleteRoleOutput{}, nil
}

func (f *fakeIAMClient) PutRolePolicy(_ context.Context, _ *iam.PutRolePolicyInput, _ ...func(*iam.Options)) (*iam.PutRolePolicyOutput, error) {
	return &iam.PutRolePolicyOutput{}, nil
}

func (f *fakeIAMClient) DeleteRolePolicy(_ context.Context, _ *iam.DeleteRolePolicyInput, _ ...func(*iam.Options)) (*iam.DeleteRolePolicyOutput, error) {
	return &iam.DeleteRolePolicyOutput{}, nil
}

func (f *fakeIAMClient) ListAttachedRolePolicies(_ context.Context, _ *iam.ListAttachedRolePoliciesInput, _ ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error) {
	if f.listAttachedOut != nil {
		return f.listAttachedOut, nil
	}
	return &iam.ListAttachedRolePoliciesOutput{}, nil
}

func ludusTaggedRole(arn string) *iam.GetRoleOutput {
	return &iam.GetRoleOutput{
		Role: &iamtypes.Role{
			Arn:  aws.String(arn),
			Tags: []iamtypes.Tag{{Key: aws.String("ManagedBy"), Value: aws.String("ludus")}},
		},
	}
}

func attachedPolicyOut() *iam.ListAttachedRolePoliciesOutput {
	return &iam.ListAttachedRolePoliciesOutput{
		AttachedPolicies: []iamtypes.AttachedPolicy{{PolicyArn: aws.String(iamPolicyARN)}},
	}
}

func TestEnsureIAMRoleRepairsMissingAttachment(t *testing.T) {
	iam := &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:existing")}
	d := &Deployer{iamClient: iam}

	got, err := d.ensureIAMRole(context.Background())
	if err != nil {
		t.Fatalf("ensureIAMRole() error = %v", err)
	}
	if got != "arn:existing" {
		t.Errorf("ensureIAMRole() = %q, want arn:existing", got)
	}
	if iam.attachCalls != 1 {
		t.Errorf("attachCalls = %d, want 1 (missing attachment repaired)", iam.attachCalls)
	}
}

func TestEnsureIAMRoleReuseSkipsAttachWhenPresent(t *testing.T) {
	iam := &fakeIAMClient{
		getRoleOut:      ludusTaggedRole("arn:existing"),
		listAttachedOut: attachedPolicyOut(),
	}
	d := &Deployer{iamClient: iam}

	if _, err := d.ensureIAMRole(context.Background()); err != nil {
		t.Fatalf("ensureIAMRole() error = %v", err)
	}
	if iam.attachCalls != 0 {
		t.Errorf("attachCalls = %d, want 0 (attachment already present)", iam.attachCalls)
	}
}

func TestDeleteIAMRoleSkipsUntaggedRole(t *testing.T) {
	iam := &fakeIAMClient{getRoleOut: &iam.GetRoleOutput{
		Role: &iamtypes.Role{Arn: aws.String("arn:foreign")},
	}}
	d := &Deployer{iamClient: iam}

	if err := d.deleteIAMRole(context.Background()); err != nil {
		t.Fatalf("deleteIAMRole() error = %v, want silent skip", err)
	}
	if iam.detachCalls != 0 || iam.deleteCalls != 0 {
		t.Errorf("untagged role touched: detach=%d delete=%d, want 0/0",
			iam.detachCalls, iam.deleteCalls)
	}
}

func TestDeleteIAMRoleDeletesTaggedRole(t *testing.T) {
	iam := &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:managed")}
	d := &Deployer{iamClient: iam}

	if err := d.deleteIAMRole(context.Background()); err != nil {
		t.Fatalf("deleteIAMRole() error = %v", err)
	}
	if iam.detachCalls != 1 || iam.deleteCalls != 1 {
		t.Errorf("detach=%d delete=%d, want 1/1 for ludus-managed role",
			iam.detachCalls, iam.deleteCalls)
	}
}

func TestDeleteIAMRoleMissingRole(t *testing.T) {
	notFound := &smithy.GenericAPIError{Code: "NoSuchEntity", Message: "not found"}
	iam := &fakeIAMClient{getRoleErr: notFound}
	d := &Deployer{iamClient: iam}

	if err := d.deleteIAMRole(context.Background()); err != nil {
		t.Fatalf("deleteIAMRole() error = %v, want silent skip on missing role", err)
	}
	if iam.detachCalls != 0 || iam.deleteCalls != 0 {
		t.Errorf("missing role touched: detach=%d delete=%d, want 0/0",
			iam.detachCalls, iam.deleteCalls)
	}
}
