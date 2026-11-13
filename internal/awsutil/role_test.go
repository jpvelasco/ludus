package awsutil

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"
	"github.com/jpvelasco/ludus/internal/tags"
)

type roleGetter struct {
	output *iam.GetRoleOutput
	err    error
	name   string
}

func (g *roleGetter) GetRole(_ context.Context, input *iam.GetRoleInput, _ ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	g.name = aws.ToString(input.RoleName)
	return g.output, g.err
}

func TestInspectOwnedRoleReadFailures(t *testing.T) {
	for _, tt := range []struct {
		name   string
		output *iam.GetRoleOutput
		code   string
		want   string
	}{
		{"missing", nil, "NoSuchEntity", ""},
		{"access denied", nil, "AccessDenied", "inspecting IAM role"},
		{"unknown not found", nil, "NotFound", "inspecting IAM role"},
		{"nil output", nil, "", "missing role ARN"},
		{"nil role", &iam.GetRoleOutput{}, "", "missing role ARN"},
		{"empty ARN", &iam.GetRoleOutput{Role: &iamtypes.Role{}}, "", "missing role ARN"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &roleGetter{output: tt.output}
			if tt.code != "" {
				client.err = &smithy.GenericAPIError{Code: tt.code}
			}
			role, reason, err := InspectOwnedRole(context.Background(), client, "example", ownedRoleTags(), "gamelift", "")
			got := ""
			if err != nil {
				got = err.Error()
			}
			if role != nil || reason != "" || !strings.Contains(got, tt.want) || (err != nil) != (tt.want != "") {
				t.Fatalf("inspection = %v, %q, %v; want %q", role, reason, err, tt.want)
			}
		})
	}
}

func ownedRoleTags() map[string]string {
	return map[string]string{"ManagedBy": "team", "Project": "game", "ludus:fleet-name": "fleet"}
}

func TestInspectOwnedRoleAttribution(t *testing.T) {
	for _, owned := range []bool{true, false} {
		t.Run(map[bool]string{true: "owned", false: "foreign"}[owned], func(t *testing.T) {
			observed := ownedRoleTags()
			if !owned {
				observed["Project"] = "other"
			}
			role := &iamtypes.Role{Arn: aws.String("arn:example"), Tags: tags.ToIAMTags(observed)}
			client := &roleGetter{output: &iam.GetRoleOutput{Role: role}}
			got, reason, err := InspectOwnedRole(context.Background(), client, "example", ownedRoleTags(), "gamelift", "")
			if err != nil || got != role || (reason == "") != owned || client.name != "example" {
				t.Fatalf("inspection = %v, %q, %v; requested %q", got, reason, err, client.name)
			}
		})
	}
}

func TestInspectOwnedRoleRejectsInvalidIdentityBeforeRead(t *testing.T) {
	_, _, err := InspectOwnedRole(context.Background(), nil, "example", nil, "gamelift", "")
	if err == nil || !strings.Contains(err.Error(), "validating IAM role ownership") {
		t.Fatalf("invalid identity error = %v", err)
	}
}
