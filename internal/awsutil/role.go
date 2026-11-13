package awsutil

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"
	"github.com/jpvelasco/ludus/internal/tags"
)

// RoleGetter is the IAM inspection needed before a direct deployer mutates a role.
type RoleGetter interface {
	GetRole(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error)
}

// InspectOwnedRole validates the expected identity and inspects the role.
// A nil role means NoSuchEntity; a nonempty reason prohibits policy or role
// mutations. Other read failures and malformed responses return an error.
func InspectOwnedRole(ctx context.Context, client RoleGetter, name string, expected map[string]string, target string) (*iamtypes.Role, string, error) {
	ownership, err := tags.NewRoleOwnership(expected, target)
	if err != nil {
		return nil, "", fmt.Errorf("validating IAM role ownership: %w", err)
	}
	out, err := client.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(name)})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchEntity" {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("inspecting IAM role: %w", err)
	}
	if out == nil || out.Role == nil || aws.ToString(out.Role.Arn) == "" {
		return nil, "", fmt.Errorf("inspecting IAM role: missing role ARN in response")
	}
	_, reason := ownership.MatchIAM(out.Role.Tags)
	return out.Role, reason, nil
}
