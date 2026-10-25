package ec2fleet

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"
)

// fakeIAMClient replays canned IAM responses and records calls so the
// ensure/repair/delete guards can be tested hermetically.
type fakeIAMClient struct {
	getRoleOut       *iam.GetRoleOutput
	getRoleErr       error
	listAttachedOut  *iam.ListAttachedRolePoliciesOutput
	listAttachedErr  error
	getRolePolicyOut *iam.GetRolePolicyOutput
	getRolePolicyErr error
	putRolePolicyErr error

	getRoleCalls     int
	attachCalls      int
	detachCalls      int
	deleteCalls      int
	putRolePolicyCnt int
	putRolePolicyIn  *iam.PutRolePolicyInput
}

func notFoundPolicyErr() error {
	return &smithy.GenericAPIError{Code: "NoSuchEntity", Message: "policy not found"}
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

func (f *fakeIAMClient) PutRolePolicy(_ context.Context, in *iam.PutRolePolicyInput, _ ...func(*iam.Options)) (*iam.PutRolePolicyOutput, error) {
	f.putRolePolicyCnt++
	f.putRolePolicyIn = in
	if f.putRolePolicyErr != nil {
		return nil, f.putRolePolicyErr
	}
	return &iam.PutRolePolicyOutput{}, nil
}

func (f *fakeIAMClient) GetRolePolicy(_ context.Context, _ *iam.GetRolePolicyInput, _ ...func(*iam.Options)) (*iam.GetRolePolicyOutput, error) {
	if f.getRolePolicyErr != nil {
		return nil, f.getRolePolicyErr
	}
	if f.getRolePolicyOut != nil {
		return f.getRolePolicyOut, nil
	}
	return nil, notFoundPolicyErr()
}

func (f *fakeIAMClient) DeleteRolePolicy(_ context.Context, _ *iam.DeleteRolePolicyInput, _ ...func(*iam.Options)) (*iam.DeleteRolePolicyOutput, error) {
	return &iam.DeleteRolePolicyOutput{}, nil
}

func (f *fakeIAMClient) ListAttachedRolePolicies(_ context.Context, _ *iam.ListAttachedRolePoliciesInput, _ ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error) {
	if f.listAttachedErr != nil {
		return nil, f.listAttachedErr
	}
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

// TestS3BuildAccessPolicy pins the least-privilege S3 resource: the default
// keeps the auto-created ludus-builds-* wildcard, a custom build bucket gets
// a resource scoped to exactly that bucket so GameLift can read the archive.
func TestS3BuildAccessPolicy(t *testing.T) {
	tests := []struct {
		name         string
		bucket       string
		wantResource string
	}{
		{"default bucket keeps the ludus-builds wildcard", "", "arn:aws:s3:::ludus-builds-*/*"},
		{"custom bucket scopes the resource exactly", "example-custom-build-storage", "arn:aws:s3:::example-custom-build-storage/*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := s3BuildAccessPolicy(tt.bucket)
			if !strings.Contains(doc, tt.wantResource) {
				t.Errorf("s3BuildAccessPolicy(%q) = %s, want resource %q", tt.bucket, doc, tt.wantResource)
			}
			for _, action := range []string{"s3:GetObject", "s3:GetObjectVersion"} {
				if !strings.Contains(doc, action) {
					t.Errorf("s3BuildAccessPolicy(%q) missing action %s", tt.bucket, action)
				}
			}
		})
	}
}

// TestPolicyNeedsUpdate pins the repair decision: a stored document that
// differs from the wanted one (or is missing/malformed) must trigger a
// PutRolePolicy, while equivalent encodings must not.
func TestPolicyNeedsUpdate(t *testing.T) {
	want := s3BuildAccessPolicy("example-custom-build-storage")
	tests := []struct {
		name    string
		current string
		wantUpd bool
	}{
		{"missing document needs update", "", true},
		{"identical document is kept", want, false},
		{"URL-encoded equivalent is kept", url.QueryEscape(want), false},
		{"invalid percent-escape needs update", "%zz broken", true},
		{"different resource needs update", s3BuildAccessPolicy("other-bucket"), true},
		{"malformed document needs update", "not json", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := policyNeedsUpdate(tt.current, want); got != tt.wantUpd {
				t.Errorf("policyNeedsUpdate(%q) = %v, want %v", tt.current, got, tt.wantUpd)
			}
		})
	}
}

// TestEnsureIAMRoleRepairsS3PolicyForCustomBucket is the regression test for
// custom build buckets being excluded from the role's S3 read policy: an
// existing role carrying the default policy must be repaired to grant access
// to the configured custom bucket.
func TestEnsureIAMRoleRepairsS3PolicyForCustomBucket(t *testing.T) {
	iam := &fakeIAMClient{
		getRoleOut:       ludusTaggedRole("arn:existing"),
		getRolePolicyOut: &iam.GetRolePolicyOutput{PolicyDocument: aws.String(s3BuildAccessPolicy(""))},
	}
	d := &Deployer{iamClient: iam, opts: DeployOptions{S3Bucket: "example-custom-build-storage"}}

	got, err := d.ensureIAMRole(context.Background())
	if err != nil {
		t.Fatalf("ensureIAMRole() error = %v", err)
	}
	if got != "arn:existing" {
		t.Errorf("ensureIAMRole() = %q, want arn:existing", got)
	}
	if iam.putRolePolicyCnt != 1 {
		t.Fatalf("PutRolePolicy calls = %d, want 1 (stale policy repaired)", iam.putRolePolicyCnt)
	}
	in := iam.putRolePolicyIn
	if aws.ToString(in.PolicyName) != s3BuildPolicyName {
		t.Errorf("PolicyName = %q, want %q", aws.ToString(in.PolicyName), s3BuildPolicyName)
	}
	if !strings.Contains(aws.ToString(in.PolicyDocument), "arn:aws:s3:::example-custom-build-storage/*") {
		t.Errorf("PolicyDocument = %s, want the custom bucket resource", aws.ToString(in.PolicyDocument))
	}
}

// TestEnsureIAMRoleKeepsMatchingS3Policy pins idempotence: a role whose inline
// policy already matches the configured bucket must not be rewritten.
func TestEnsureIAMRoleKeepsMatchingS3Policy(t *testing.T) {
	iam := &fakeIAMClient{
		getRoleOut:       ludusTaggedRole("arn:existing"),
		getRolePolicyOut: &iam.GetRolePolicyOutput{PolicyDocument: aws.String(s3BuildAccessPolicy("example-custom-build-storage"))},
	}
	d := &Deployer{iamClient: iam, opts: DeployOptions{S3Bucket: "example-custom-build-storage"}}

	if _, err := d.ensureIAMRole(context.Background()); err != nil {
		t.Fatalf("ensureIAMRole() error = %v", err)
	}
	if iam.putRolePolicyCnt != 0 {
		t.Errorf("PutRolePolicy calls = %d, want 0 (policy already correct)", iam.putRolePolicyCnt)
	}
}

// TestEnsureIAMRoleAddsS3PolicyWhenMissing covers the repair path for roles
// predating the inline policy entirely.
func TestEnsureIAMRoleAddsS3PolicyWhenMissing(t *testing.T) {
	iam := &fakeIAMClient{
		getRoleOut:       ludusTaggedRole("arn:existing"),
		getRolePolicyErr: notFoundPolicyErr(),
	}
	d := &Deployer{iamClient: iam, opts: DeployOptions{S3Bucket: "example-custom-build-storage"}}

	if _, err := d.ensureIAMRole(context.Background()); err != nil {
		t.Fatalf("ensureIAMRole() error = %v", err)
	}
	if iam.putRolePolicyCnt != 1 {
		t.Errorf("PutRolePolicy calls = %d, want 1 (missing policy created)", iam.putRolePolicyCnt)
	}
}

// TestEnsureIAMRoleCreatesRoleWithCustomBucketPolicy covers the fresh-role
// path: the inline policy must be scoped to the configured custom bucket from
// the start.
func TestEnsureIAMRoleCreatesRoleWithCustomBucketPolicy(t *testing.T) {
	orig := iamPropagationWait
	iamPropagationWait = 0
	t.Cleanup(func() { iamPropagationWait = orig })

	notFound := &smithy.GenericAPIError{Code: "NoSuchEntity", Message: "role not found"}
	iam := &fakeIAMClient{getRoleErr: notFound}
	d := &Deployer{iamClient: iam, opts: DeployOptions{S3Bucket: "example-custom-build-storage"}}

	got, err := d.ensureIAMRole(context.Background())
	if err != nil {
		t.Fatalf("ensureIAMRole() error = %v", err)
	}
	if got != "arn:created" {
		t.Errorf("ensureIAMRole() = %q, want arn:created", got)
	}
	if iam.putRolePolicyCnt != 1 {
		t.Fatalf("PutRolePolicy calls = %d, want 1", iam.putRolePolicyCnt)
	}
	if !strings.Contains(aws.ToString(iam.putRolePolicyIn.PolicyDocument), "arn:aws:s3:::example-custom-build-storage/*") {
		t.Errorf("PolicyDocument = %s, want the custom bucket resource", aws.ToString(iam.putRolePolicyIn.PolicyDocument))
	}
}

// TestEnsureIAMRoleReportsS3PolicyReadError covers the guard around reading
// the inline policy on an existing role.
func TestEnsureIAMRoleReportsS3PolicyReadError(t *testing.T) {
	denied := &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
	iam := &fakeIAMClient{
		getRoleOut:       ludusTaggedRole("arn:existing"),
		getRolePolicyErr: denied,
	}
	d := &Deployer{iamClient: iam}

	_, err := d.ensureIAMRole(context.Background())
	if err == nil || !strings.Contains(err.Error(), "verifying S3 build access policy") {
		t.Fatalf("ensureIAMRole() error = %v, want S3 policy verification wrap", err)
	}
	if iam.putRolePolicyCnt != 0 {
		t.Errorf("PutRolePolicy calls = %d, want 0 on read failure", iam.putRolePolicyCnt)
	}
}

// TestEnsureS3BuildAccessPolicyReportsPutError covers the write guard of the
// inline-policy repair.
func TestEnsureS3BuildAccessPolicyReportsPutError(t *testing.T) {
	denied := &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
	iam := &fakeIAMClient{putRolePolicyErr: denied}
	d := &Deployer{iamClient: iam}

	err := d.ensureS3BuildAccessPolicy(context.Background())
	if err == nil || !strings.Contains(err.Error(), "adding S3 access policy") {
		t.Fatalf("ensureS3BuildAccessPolicy() error = %v, want put failure wrap", err)
	}
}
