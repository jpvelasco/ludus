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

// TestRunAll_SkipAWS pins RunAll's SkipAWS gate: the AWS CLI and AWS
// credentials checks run by default and are omitted when SkipAWS is set (e.g.
// a binary-export pipeline that never talks to AWS).
func TestRunAll_SkipAWS(t *testing.T) {
	// aws is stubbed so the credentials check resolves offline when it runs.
	testsupport.FakeTool(t, "aws", testsupport.ToolBehavior{
		Stdout: `{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/test"}`,
	})

	tests := []struct {
		name    string
		skipAWS bool
		wantAWS bool
	}{
		{name: "aws checks run by default", wantAWS: true},
		{name: "aws checks omitted with skip", skipAWS: true, wantAWS: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewChecker("", "", false, nil)
			c.SkipAWS = tt.skipAWS

			hasAWS := false
			for _, res := range c.RunAll() {
				if res.Name == "AWS CLI" || res.Name == "AWS Credentials" {
					hasAWS = true
				}
			}
			if hasAWS != tt.wantAWS {
				t.Errorf("RunAll() reports aws checks = %v, want %v", hasAWS, tt.wantAWS)
			}
		})
	}
}
