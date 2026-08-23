package gamelift

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

// ensureIAMRole creates the GameLift fleet IAM role if it doesn't exist, returns the role ARN.
// A pre-existing role is verified and its policy attachment repaired
// idempotently, so a run crashed between CreateRole and AttachRolePolicy
// cannot poison every later deploy with opaque authorization failures.
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
		Description:              aws.String("IAM role for Ludus GameLift container fleet"),
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

	// IAM changes take ~10s to propagate globally before GameLift can assume the role.
	time.Sleep(d.iamPropagationDelay)

	return aws.ToString(createOut.Role.Arn), nil
}

// ensureRolePolicyAttached verifies that the ludus policy is attached to the
// fixed-name role and attaches it when missing.
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
	// it is tagged as ludus-managed. Anything else may belong to another
	// deployment or operator.
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
