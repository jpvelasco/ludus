package ec2fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/tags"
)

// s3BuildPolicyName is the inline role policy granting GameLift read access to
// the build archive.
const s3BuildPolicyName = "LudusS3BuildAccess"

// iamPropagationWait is how long ensureIAMRole waits for a freshly created
// role to propagate. It is a variable so tests can shorten it.
var iamPropagationWait = 10 * time.Second

// ensureIAMRole creates the GameLift EC2 fleet IAM role if it doesn't exist.
// A pre-existing role is verified and its managed-policy attachment and inline
// S3 build-access policy repaired idempotently (see the gamelift twin for
// rationale).
func (d *Deployer) ensureIAMRole(ctx context.Context) (string, error) {
	getOut, err := d.iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(iamRoleName),
	})
	if err == nil {
		if err := d.ensureRolePolicyAttached(ctx); err != nil {
			return "", fmt.Errorf("verifying policy attachment on existing %s: %w", iamRoleName, err)
		}
		if err := d.ensureS3BuildAccessPolicy(ctx); err != nil {
			return "", fmt.Errorf("verifying S3 build access policy on existing %s: %w", iamRoleName, err)
		}
		return aws.ToString(getOut.Role.Arn), nil
	}

	assumeRolePolicy := `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"Service": "gamelift.amazonaws.com"},
    "Action": "sts:AssumeRole"
  }]
}`

	createOut, err := d.iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(iamRoleName),
		AssumeRolePolicyDocument: aws.String(assumeRolePolicy),
		Description:              aws.String("IAM role for Ludus GameLift EC2 fleet"),
		Tags:                     tags.ToIAMTags(d.resourceTags()),
	})
	if err != nil {
		return "", fmt.Errorf("creating IAM role: %w", err)
	}

	_, err = d.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(iamRoleName),
		PolicyArn: aws.String(iamPolicyARN),
	})
	if err != nil {
		return "", fmt.Errorf("attaching policy to role: %w", err)
	}

	// EC2 managed builds require S3 read access for GameLift to download
	// the build archive; the policy is scoped to the resolved bucket.
	if err := d.ensureS3BuildAccessPolicy(ctx); err != nil {
		return "", err
	}

	// Wait for IAM propagation
	time.Sleep(iamPropagationWait)

	return aws.ToString(createOut.Role.Arn), nil
}

// s3BuildAccessPolicy returns the inline policy document granting GameLift
// read access to the deployment's build archive. A custom bucket gets a
// least-privilege resource for exactly that bucket; the empty (auto-created)
// case keeps the ludus-builds-* wildcard, which covers every account-derived
// bucket name.
func s3BuildAccessPolicy(bucket string) string {
	resource := "arn:aws:s3:::ludus-builds-*/*"
	if bucket != "" {
		resource = fmt.Sprintf("arn:aws:s3:::%s/*", bucket)
	}
	return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["s3:GetObject", "s3:GetObjectVersion"],
    "Resource": %q
  }]
}`, resource)
}

// ensureS3BuildAccessPolicy idempotently maintains the inline S3 policy that
// lets GameLift download the build archive. Roles deployed before a custom
// bucket was configured (or before the policy existed) are repaired in place.
func (d *Deployer) ensureS3BuildAccessPolicy(ctx context.Context) error {
	want := s3BuildAccessPolicy(d.opts.S3Bucket)
	out, err := d.iamClient.GetRolePolicy(ctx, &iam.GetRolePolicyInput{
		RoleName:   aws.String(iamRoleName),
		PolicyName: aws.String(s3BuildPolicyName),
	})
	if err == nil && !policyNeedsUpdate(aws.ToString(out.PolicyDocument), want) {
		return nil
	}
	if err != nil && !awsutil.IsNotFound(err) {
		return fmt.Errorf("reading inline S3 policy: %w", err)
	}

	_, err = d.iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(iamRoleName),
		PolicyName:     aws.String(s3BuildPolicyName),
		PolicyDocument: aws.String(want),
	})
	if err != nil {
		return fmt.Errorf("adding S3 access policy: %w", err)
	}
	return nil
}

// policyNeedsUpdate reports whether the role's stored inline policy document
// differs from want. AWS returns the document URL-encoded and re-serialized,
// so both sides are normalized through a JSON round-trip before comparing;
// a missing or malformed document always needs an update.
func policyNeedsUpdate(current, want string) bool {
	if current == "" {
		return true
	}
	return normalizePolicyJSON(current) != normalizePolicyJSON(want)
}

// normalizePolicyJSON canonicalizes a policy document for comparison.
// Undecodable input normalizes to "" so it always compares unequal.
func normalizePolicyJSON(doc string) string {
	if decoded, err := url.QueryUnescape(doc); err == nil {
		doc = decoded
	}
	var parsed any
	if err := json.Unmarshal([]byte(doc), &parsed); err != nil {
		return ""
	}
	canonical, err := json.Marshal(parsed)
	if err != nil {
		return ""
	}
	return string(canonical)
}

// ensureRolePolicyAttached verifies that the ludus managed policy is attached
// to the fixed-name role and attaches it when missing. The inline S3 policy is
// only added at create time; ListAttachedRolePolicies covers the managed one.
func (d *Deployer) ensureRolePolicyAttached(ctx context.Context) error {
	out, err := d.iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(iamRoleName),
	})
	if err != nil {
		return err
	}
	for _, p := range out.AttachedPolicies {
		if aws.ToString(p.PolicyArn) == iamPolicyARN {
			return nil
		}
	}

	fmt.Printf("  Policy %s missing on existing role, attaching...\n", iamPolicyARN)
	_, err = d.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(iamRoleName),
		PolicyArn: aws.String(iamPolicyARN),
	})
	return err
}

func (d *Deployer) deleteIAMRole(ctx context.Context) error {
	fmt.Println("Deleting IAM role...")

	// Ownership guard: the role name is account-global, so only delete it when
	// it is tagged as ludus-managed (see the gamelift twin for rationale).
	getOut, err := d.iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(iamRoleName),
	})
	if err != nil {
		if awsutil.IsNotFound(err) {
			fmt.Println("IAM role not found, skipping.")
			return nil
		}
		return fmt.Errorf("inspecting IAM role before deletion: %w", err)
	}
	if !roleIsLudusManaged(getOut.Role.Tags) {
		fmt.Printf("Skipping IAM role %s: it is not tagged ManagedBy=ludus and may not belong to this deployment.\n", iamRoleName)
		return nil
	}

	_, err = d.iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
		RoleName:  aws.String(iamRoleName),
		PolicyArn: aws.String(iamPolicyARN),
	})
	if err != nil && !awsutil.IsNotFound(err) {
		return fmt.Errorf("detaching policy from role: %w", err)
	}

	// Remove S3 access inline policy
	_, _ = d.iamClient.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
		RoleName:   aws.String(iamRoleName),
		PolicyName: aws.String(s3BuildPolicyName),
	})

	_, err = d.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(iamRoleName),
	})
	if err != nil {
		if awsutil.IsNotFound(err) {
			fmt.Println("IAM role not found, skipping.")
			return nil
		}
		return fmt.Errorf("deleting IAM role: %w", err)
	}

	fmt.Println("IAM role deleted.")
	return nil
}

// roleIsLudusManaged reports whether the role carries a ManagedBy=ludus tag.
func roleIsLudusManaged(roleTags []iamtypes.Tag) bool {
	for _, t := range roleTags {
		if aws.ToString(t.Key) == "ManagedBy" && aws.ToString(t.Value) == "ludus" {
			return true
		}
	}
	return false
}
