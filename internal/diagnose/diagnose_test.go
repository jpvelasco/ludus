package diagnose

import (
	"errors"
	"strings"
	"testing"
)

type diagnoseTestCase struct {
	name         string
	err          error
	context      string // operation or target name
	wantNil      bool
	wantContains []string
	wantNotHints bool
}

func assertDiagnoseResult(t *testing.T, got error, tc diagnoseTestCase) {
	t.Helper()
	if tc.wantNil {
		if got != nil {
			t.Errorf("got error %v, want nil", got)
		}
		return
	}
	if got == nil {
		t.Fatal("got nil, want error")
	}
	errMsg := got.Error()
	if tc.wantNotHints {
		if strings.Contains(errMsg, "Suggestions:") {
			t.Errorf("error should not contain hints, got: %s", errMsg)
		}
	}
	for _, want := range tc.wantContains {
		if !strings.Contains(errMsg, want) {
			t.Errorf("error = %q, want to contain %q", errMsg, want)
		}
	}
}

var awsErrorTests = []diagnoseTestCase{
	{name: "nil error returns nil", err: nil, context: "test operation", wantNil: true},
	{name: "unknown error wrapped without hints", err: errors.New("some random error"), context: "test operation",
		wantContains: []string{"test operation:", "some random error"}, wantNotHints: true},
	{name: "ExpiredTokenException includes aws sso login hint", err: errors.New("ExpiredTokenException: token expired"), context: "describe fleets",
		wantContains: []string{"describe fleets", "Suggestions:", "aws sso login"}},
	{name: "AccessDeniedException includes IAM permissions hint", err: errors.New("AccessDeniedException: User not authorized"), context: "create fleet",
		wantContains: []string{"create fleet", "Suggestions:", "IAM permissions"}},
	{name: "AccessDenied includes IAM policy hint", err: errors.New("AccessDenied: forbidden"), context: "list fleets",
		wantContains: []string{"list fleets", "Suggestions:", "IAM policy"}},
	{name: "NoCredentialProviders includes aws configure hint", err: errors.New("NoCredentialProviders: no credentials"), context: "describe regions",
		wantContains: []string{"describe regions", "Suggestions:", "aws configure"}},
}

var deployErrorTests = []diagnoseTestCase{
	{name: "nil error returns nil", err: nil, context: "gamelift", wantNil: true},
	{name: "unknown error wrapped without hints", err: errors.New("unknown failure"), context: "gamelift",
		wantContains: []string{"deploy gamelift failed:", "unknown failure"}, wantNotHints: true},
	{name: "fleet is in ERROR includes hint", err: errors.New("fleet is in ERROR state"), context: "gamelift",
		wantContains: []string{"deploy gamelift failed", "Suggestions:", "check AWS GameLift console"}},
	{name: "timed out waiting includes hint", err: errors.New("timed out waiting for fleet to be ready"), context: "stack",
		wantContains: []string{"deploy stack failed", "Suggestions:", "deployment timed out"}},
	{name: "AccessDenied includes AWS hint in deploy error", err: errors.New("AccessDenied: insufficient permissions"), context: "ec2",
		wantContains: []string{"deploy ec2 failed", "Suggestions:", "IAM policy"}},
	{name: "ExpiredToken includes AWS hint in deploy error", err: errors.New("ExpiredToken: session expired"), context: "anywhere",
		wantContains: []string{"deploy anywhere failed", "Suggestions:", "aws sso login"}},
	{name: "ConflictException includes deploy hint", err: errors.New("ConflictException: resource exists"), context: "gamelift",
		wantContains: []string{"deploy gamelift failed", "Suggestions:", "ludus deploy destroy"}},
}

var containerErrorTests = []diagnoseTestCase{
	{name: "nil error returns nil", err: nil, context: "docker build", wantNil: true},
	{name: "unknown error wrapped without hints", err: errors.New("random docker error"), context: "docker build",
		wantContains: []string{"docker build:", "random docker error"}, wantNotHints: true},
	{name: "no space left on device includes prune hint", err: errors.New("write error: no space left on device"), context: "docker build",
		wantContains: []string{"docker build", "Suggestions:", "docker system prune"}},
	{name: "Cannot connect to the Docker daemon includes startup hint", err: errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock"), context: "docker push",
		wantContains: []string{"docker push", "Suggestions:", "Docker not running", "Docker Desktop"}},
	{name: "ECR token expired includes re-auth hint", err: errors.New("denied: Your authorization token has expired"), context: "docker push",
		wantContains: []string{"docker push", "Suggestions:", "ECR login expired", "re-authenticates automatically"}},
	{name: "toomanyrequests includes rate limit hint", err: errors.New("toomanyrequests: Docker Hub rate limit"), context: "docker pull",
		wantContains: []string{"docker pull", "Suggestions:", "rate limit", "wait 15 minutes"}},
	{name: "COPY failed includes verification hint", err: errors.New("COPY failed: stat /files: no such file"), context: "docker build",
		wantContains: []string{"docker build", "Suggestions:", "COPY failed", "verify the server build directory"}},
}

func TestAWSError(t *testing.T) {
	for _, tt := range awsErrorTests {
		t.Run(tt.name, func(t *testing.T) {
			assertDiagnoseResult(t, AWSError(tt.err, tt.context), tt)
		})
	}
}

func TestDeployError(t *testing.T) {
	for _, tt := range deployErrorTests {
		t.Run(tt.name, func(t *testing.T) {
			assertDiagnoseResult(t, DeployError(tt.err, tt.context), tt)
		})
	}
}

func TestContainerError(t *testing.T) {
	for _, tt := range containerErrorTests {
		t.Run(tt.name, func(t *testing.T) {
			assertDiagnoseResult(t, ContainerError(tt.err, tt.context), tt)
		})
	}
}
