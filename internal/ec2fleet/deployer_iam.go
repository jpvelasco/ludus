package ec2fleet

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/tags"
)

// ensureIAMRole creates the GameLift EC2 fleet IAM role if it doesn't exist.
// A pre-existing role is verified and its managed-policy attachment repaired
// idempotently (see the gamelift twin for rationale).
func (d *Deployer) ensureIAMRole(ctx context.Context) (string, error) {
	getOut, err := d.iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(iamRoleName),
	})
	if err == nil {
		if err := d.ensureRolePolicyAttached(ctx); err != nil {
			return "", fmt.Errorf("verifying policy attachment on existing %s: %w", iamRoleName, err)
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
	// the build archive. The GameLiftContainerFleetPolicy only covers
	// container fleets, so we add an inline policy for S3.
	s3Policy := `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["s3:GetObject", "s3:GetObjectVersion"],
    "Resource": "arn:aws:s3:::ludus-builds-*/*"
  }]
}`
	_, err = d.iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(iamRoleName),
		PolicyName:     aws.String("LudusS3BuildAccess"),
		PolicyDocument: aws.String(s3Policy),
	})
	if err != nil {
		return "", fmt.Errorf("adding S3 access policy: %w", err)
	}

	// Wait for IAM propagation
	time.Sleep(10 * time.Second)

	return aws.ToString(createOut.Role.Arn), nil
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
		PolicyName: aws.String("LudusS3BuildAccess"),
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
