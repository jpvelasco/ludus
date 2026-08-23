package gamelift

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// fakeIAMClient implements iamAPI with canned responses and call capture.
type fakeIAMClient struct {
	getRoleOut    *iam.GetRoleOutput
	getRoleErr    error
	createRoleOut *iam.CreateRoleOutput
	createRoleErr error
	attachErr     error
	detachErr     error
	deleteRoleErr error

	listAttachedOut *iam.ListAttachedRolePoliciesOutput
	listAttachedErr error

	getRoleCalls    int
	createRoleCalls int
	attachCalls     int
	detachCalls     int
	deleteRoleCalls int
	listAttachCalls int

	createRoleInput *iam.CreateRoleInput
	attachInput     *iam.AttachRolePolicyInput
}

func (f *fakeIAMClient) GetRole(_ context.Context, _ *iam.GetRoleInput, _ ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	f.getRoleCalls++
	return f.getRoleOut, f.getRoleErr
}

func (f *fakeIAMClient) CreateRole(_ context.Context, in *iam.CreateRoleInput, _ ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
	f.createRoleCalls++
	f.createRoleInput = in
	return f.createRoleOut, f.createRoleErr
}

func (f *fakeIAMClient) AttachRolePolicy(_ context.Context, in *iam.AttachRolePolicyInput, _ ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
	f.attachCalls++
	f.attachInput = in
	return &iam.AttachRolePolicyOutput{}, f.attachErr
}

func (f *fakeIAMClient) DetachRolePolicy(_ context.Context, _ *iam.DetachRolePolicyInput, _ ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error) {
	f.detachCalls++
	return &iam.DetachRolePolicyOutput{}, f.detachErr
}

func (f *fakeIAMClient) DeleteRole(_ context.Context, _ *iam.DeleteRoleInput, _ ...func(*iam.Options)) (*iam.DeleteRoleOutput, error) {
	f.deleteRoleCalls++
	return &iam.DeleteRoleOutput{}, f.deleteRoleErr
}

func (f *fakeIAMClient) ListAttachedRolePolicies(_ context.Context, _ *iam.ListAttachedRolePoliciesInput, _ ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error) {
	f.listAttachCalls++
	if f.listAttachedErr != nil {
		return nil, f.listAttachedErr
	}
	if f.listAttachedOut != nil {
		return f.listAttachedOut, nil
	}
	return &iam.ListAttachedRolePoliciesOutput{}, nil
}

func iamRoleOutput(arn string) *iam.GetRoleOutput {
	return &iam.GetRoleOutput{Role: &iamtypes.Role{Arn: aws.String(arn)}}
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

func newTestIAMDeployer(client *fakeIAMClient) *Deployer {
	return &Deployer{
		opts:                DeployOptions{},
		iamClient:           client,
		iamPropagationDelay: 0,
	}
}

func TestEnsureIAMRole(t *testing.T) {
	tests := []struct {
		name        string
		iam         *fakeIAMClient
		wantARN     string
		wantErrSub  string
		wantCreates int
		wantAttach  int
	}{
		{
			name: "reuses existing role",
			iam: &fakeIAMClient{
				getRoleOut:      iamRoleOutput("arn:existing"),
				listAttachedOut: attachedPolicyOut(),
			},
			wantARN: "arn:existing",
		},
		{
			name: "creates role and attaches policy",
			iam: &fakeIAMClient{
				getRoleErr:    &cgdAPIError{code: "NotFoundException"},
				createRoleOut: &iam.CreateRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:created")}},
			},
			wantARN:     "arn:created",
			wantCreates: 1,
			wantAttach:  1,
		},
		{
			name:        "create role error",
			iam:         &fakeIAMClient{getRoleErr: &cgdAPIError{code: "NotFoundException"}, createRoleErr: &cgdAPIError{code: "LimitExceededException"}},
			wantErrSub:  "creating IAM role",
			wantCreates: 1,
		},
		{
			name: "attach policy error",
			iam: &fakeIAMClient{
				getRoleErr:    &cgdAPIError{code: "NotFoundException"},
				createRoleOut: &iam.CreateRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:created")}},
				attachErr:     &cgdAPIError{code: "AccessDeniedException"},
			},
			wantErrSub:  "attaching policy to role",
			wantCreates: 1,
			wantAttach:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestIAMDeployer(tt.iam)
			got, err := d.ensureIAMRole(context.Background())
			switch {
			case tt.wantErrSub != "":
				assertErrorContains(t, err, tt.wantErrSub)
			case err != nil:
				t.Fatalf("ensureIAMRole() error = %v", err)
			case got != tt.wantARN:
				t.Errorf("ensureIAMRole() = %q, want %q", got, tt.wantARN)
			}
			if tt.iam.createRoleCalls != tt.wantCreates || tt.iam.attachCalls != tt.wantAttach {
				t.Errorf("iam calls = create:%d attach:%d, want create:%d attach:%d",
					tt.iam.createRoleCalls, tt.iam.attachCalls, tt.wantCreates, tt.wantAttach)
			}
		})
	}
}

func TestEnsureIAMRoleWiresFleetTags(t *testing.T) {
	iam := &fakeIAMClient{
		getRoleErr:    &cgdAPIError{code: "NotFoundException"},
		createRoleOut: &iam.CreateRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:created")}},
	}
	d := &Deployer{
		opts:                DeployOptions{FleetName: "tagged-fleet", Tags: map[string]string{"ManagedBy": "ludus"}},
		iamClient:           iam,
		iamPropagationDelay: 0,
	}

	if _, err := d.ensureIAMRole(context.Background()); err != nil {
		t.Fatalf("ensureIAMRole() error = %v", err)
	}
	if got := aws.ToString(iam.attachInput.PolicyArn); got != iamPolicyARN {
		t.Errorf("PolicyArn = %q, want %q", got, iamPolicyARN)
	}
	roleName := aws.ToString(iam.createRoleInput.RoleName)
	if roleName != iamRoleName {
		t.Errorf("RoleName = %q, want %q", roleName, iamRoleName)
	}
}

func TestDeleteIAMRole(t *testing.T) {
	tests := []struct {
		name         string
		iam          *fakeIAMClient
		wantErrSub   string
		wantDetaches int
		wantDeletes  int
	}{
		{
			name:         "success",
			iam:          &fakeIAMClient{getRoleOut: ludusTaggedRole("arn:managed")},
			wantDetaches: 1,
			wantDeletes:  1,
		},
		{
			name: "missing role skipped",
			iam: &fakeIAMClient{
				getRoleOut:    ludusTaggedRole("arn:managed"),
				detachErr:     &cgdAPIError{code: "NoSuchEntity"},
				deleteRoleErr: &cgdAPIError{code: "NoSuchEntity"},
			},
			wantDetaches: 1,
			wantDeletes:  1,
		},
		{
			name: "detach error propagates",
			iam: &fakeIAMClient{
				getRoleOut: ludusTaggedRole("arn:managed"),
				detachErr:  &cgdAPIError{code: "AccessDeniedException"},
			},
			wantErrSub:   "detaching policy from role",
			wantDetaches: 1,
		},
		{
			name: "delete error propagates",
			iam: &fakeIAMClient{
				getRoleOut:    ludusTaggedRole("arn:managed"),
				deleteRoleErr: &cgdAPIError{code: "AccessDeniedException"},
			},
			wantErrSub:   "deleting IAM role",
			wantDetaches: 1,
			wantDeletes:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestIAMDeployer(tt.iam)
			err := d.deleteIAMRole(context.Background())
			if tt.wantErrSub != "" {
				assertErrorContains(t, err, tt.wantErrSub)
			} else if err != nil {
				t.Fatalf("deleteIAMRole() error = %v", err)
			}
			if tt.iam.detachCalls != tt.wantDetaches || tt.iam.deleteRoleCalls != tt.wantDeletes {
				t.Errorf("iam calls = detach:%d delete:%d, want detach:%d delete:%d",
					tt.iam.detachCalls, tt.iam.deleteRoleCalls, tt.wantDetaches, tt.wantDeletes)
			}
		})
	}
}

// TestEnsureIAMRoleRepairsMissingAttachment pins the #561 contract: an
// existing role left half-created by a crashed run (role present, policy
// attachment missing) must be repaired idempotently instead of silently
// reused and failing later inside fleet creation.
func TestEnsureIAMRoleRepairsMissingAttachment(t *testing.T) {
	iam := &fakeIAMClient{
		getRoleOut: iamRoleOutput("arn:existing"),
	}
	d := newTestIAMDeployer(iam)

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

// TestDeleteIAMRoleSkipsUntaggedRole pins the destroy-side guard: a role that
// is not tagged ManagedBy=ludus may belong to another deployment or operator;
// it must be left alone.
func TestDeleteIAMRoleSkipsUntaggedRole(t *testing.T) {
	iam := &fakeIAMClient{getRoleOut: iamRoleOutput("arn:foreign")}
	d := newTestIAMDeployer(iam)

	if err := d.deleteIAMRole(context.Background()); err != nil {
		t.Fatalf("deleteIAMRole() error = %v, want silent skip", err)
	}
	if iam.detachCalls != 0 || iam.deleteRoleCalls != 0 {
		t.Errorf("untagged role touched: detach=%d delete=%d, want 0/0",
			iam.detachCalls, iam.deleteRoleCalls)
	}
}
