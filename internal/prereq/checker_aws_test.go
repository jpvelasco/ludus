package prereq

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestCheckAWSCredentials_Authenticated(t *testing.T) {
	testsupport.FakeTool(t, "aws", testsupport.ToolBehavior{
		Stdout: `{"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/test"}`,
	})
	res := (&Checker{}).checkAWSCredentials()
	if !res.Passed || res.Warning || !strings.Contains(res.Message, "authenticated") {
		t.Fatalf("checkAWSCredentials() = %+v", res)
	}
}

func TestCheckAWSCredentials_UnparseableOutput(t *testing.T) {
	testsupport.FakeTool(t, "aws", testsupport.ToolBehavior{Stdout: "garbage"})
	res := (&Checker{}).checkAWSCredentials()
	if !res.Passed || !res.Warning || !strings.Contains(res.Message, "unexpected output") {
		t.Fatalf("checkAWSCredentials() = %+v", res)
	}
}

func TestCheckAWSCredentials_NotConfigured(t *testing.T) {
	testsupport.FakeTool(t, "aws", testsupport.ToolBehavior{ExitCode: 1})
	res := (&Checker{}).checkAWSCredentials()
	if !res.Passed || !res.Warning || !strings.Contains(res.Message, "not configured or expired") {
		t.Fatalf("checkAWSCredentials() = %+v", res)
	}
}

func TestCheckAWSCredentials_CLINotInstalled(t *testing.T) {
	t.Setenv("PATH", "")
	res := (&Checker{}).checkAWSCredentials()
	if !res.Passed || !res.Warning || !strings.Contains(res.Message, "AWS CLI not installed") {
		t.Fatalf("checkAWSCredentials() = %+v", res)
	}
}
