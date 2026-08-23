package ec2fleet

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/smithy-go"
)

func TestEnsureIAMRoleListAttachError(t *testing.T) {
	denied := &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
	iam := &fakeIAMClient{
		getRoleOut:      ludusTaggedRole("arn:existing"),
		listAttachedErr: denied,
	}
	d := &Deployer{iamClient: iam}

	_, err := d.ensureIAMRole(context.Background())
	if err == nil || !strings.Contains(err.Error(), "verifying policy attachment") {
		t.Fatalf("ensureIAMRole() error = %v, want verification wrap", err)
	}
}

func TestDeleteIAMRoleInspectError(t *testing.T) {
	denied := &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
	iam := &fakeIAMClient{getRoleErr: denied}
	d := &Deployer{iamClient: iam}

	err := d.deleteIAMRole(context.Background())
	if err == nil || !strings.Contains(err.Error(), "inspecting IAM role") {
		t.Fatalf("deleteIAMRole() error = %v, want inspect wrap", err)
	}
	if iam.detachCalls != 0 || iam.deleteCalls != 0 {
		t.Errorf("role touched despite inspect error: detach=%d delete=%d",
			iam.detachCalls, iam.deleteCalls)
	}
}
