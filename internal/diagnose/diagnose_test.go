package diagnose

import (
	"errors"
	"strings"
	"testing"
)

func TestAWSError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		operation    string
		wantNil      bool
		wantContains []string
		wantNotHints bool
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			operation: "test operation",
			wantNil:   true,
		},
		{
			name:         "unknown error wrapped without hints",
			err:          errors.New("some random error"),
			operation:    "test operation",
			wantContains: []string{"test operation:", "some random error"},
			wantNotHints: true,
		},
		{
			name:         "ExpiredTokenException includes aws sso login hint",
			err:          errors.New("ExpiredTokenException: token expired"),
			operation:    "describe fleets",
			wantContains: []string{"describe fleets", "Suggestions:", "aws sso login"},
		},
		{
			name:         "AccessDeniedException includes IAM permissions hint",
			err:          errors.New("AccessDeniedException: User not authorized"),
			operation:    "create fleet",
			wantContains: []string{"create fleet", "Suggestions:", "IAM permissions"},
		},
		{
			name:         "AccessDenied includes IAM policy hint",
			err:          errors.New("AccessDenied: forbidden"),
			operation:    "list fleets",
			wantContains: []string{"list fleets", "Suggestions:", "IAM policy"},
		},
		{
			name:         "NoCredentialProviders includes aws configure hint",
			err:          errors.New("NoCredentialProviders: no credentials"),
			operation:    "describe regions",
			wantContains: []string{"describe regions", "Suggestions:", "aws configure"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AWSError(tt.err, tt.operation)

			if tt.wantNil {
				if got != nil {
					t.Errorf("AWSError() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("AWSError() returned nil, want error")
			}

			errMsg := got.Error()

			if tt.wantNotHints {
				// Should not contain "Suggestions:"
				if strings.Contains(errMsg, "Suggestions:") {
					t.Errorf("AWSError() error should not contain hints, got: %s", errMsg)
				}
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(errMsg, want) {
					t.Errorf("AWSError() error = %q, want to contain %q", errMsg, want)
				}
			}
		})
	}
}

func TestDeployError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		target       string
		wantNil      bool
		wantContains []string
		wantNotHints bool
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			target:  "gamelift",
			wantNil: true,
		},
		{
			name:         "unknown error wrapped without hints",
			err:          errors.New("unknown failure"),
			target:       "gamelift",
			wantContains: []string{"deploy gamelift failed:", "unknown failure"},
			wantNotHints: true,
		},
		{
			name:         "fleet is in ERROR includes hint",
			err:          errors.New("fleet is in ERROR state"),
			target:       "gamelift",
			wantContains: []string{"deploy gamelift failed", "Suggestions:", "check AWS GameLift console"},
		},
		{
			name:         "timed out waiting includes hint",
			err:          errors.New("timed out waiting for fleet to be ready"),
			target:       "stack",
			wantContains: []string{"deploy stack failed", "Suggestions:", "deployment timed out"},
		},
		{
			name:         "AccessDenied includes AWS hint in deploy error",
			err:          errors.New("AccessDenied: insufficient permissions"),
			target:       "ec2",
			wantContains: []string{"deploy ec2 failed", "Suggestions:", "IAM policy"},
		},
		{
			name:         "ExpiredToken includes AWS hint in deploy error",
			err:          errors.New("ExpiredToken: session expired"),
			target:       "anywhere",
			wantContains: []string{"deploy anywhere failed", "Suggestions:", "aws sso login"},
		},
		{
			name:         "ConflictException includes deploy hint",
			err:          errors.New("ConflictException: resource exists"),
			target:       "gamelift",
			wantContains: []string{"deploy gamelift failed", "Suggestions:", "ludus deploy destroy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeployError(tt.err, tt.target)

			if tt.wantNil {
				if got != nil {
					t.Errorf("DeployError() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("DeployError() returned nil, want error")
			}

			errMsg := got.Error()

			if tt.wantNotHints {
				if strings.Contains(errMsg, "Suggestions:") {
					t.Errorf("DeployError() error should not contain hints, got: %s", errMsg)
				}
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(errMsg, want) {
					t.Errorf("DeployError() error = %q, want to contain %q", errMsg, want)
				}
			}
		})
	}
}

func TestContainerError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		operation    string
		wantNil      bool
		wantContains []string
		wantNotHints bool
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			operation: "docker build",
			wantNil:   true,
		},
		{
			name:         "unknown error wrapped without hints",
			err:          errors.New("random docker error"),
			operation:    "docker build",
			wantContains: []string{"docker build:", "random docker error"},
			wantNotHints: true,
		},
		{
			name:         "no space left on device includes prune hint",
			err:          errors.New("write error: no space left on device"),
			operation:    "docker build",
			wantContains: []string{"docker build", "Suggestions:", "docker system prune"},
		},
		{
			name:         "Cannot connect to the Docker daemon includes startup hint",
			err:          errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock"),
			operation:    "docker push",
			wantContains: []string{"docker push", "Suggestions:", "Docker not running", "Docker Desktop"},
		},
		{
			name:         "ECR token expired includes re-auth hint",
			err:          errors.New("denied: Your authorization token has expired"),
			operation:    "docker push",
			wantContains: []string{"docker push", "Suggestions:", "ECR login expired", "re-authenticates automatically"},
		},
		{
			name:         "toomanyrequests includes rate limit hint",
			err:          errors.New("toomanyrequests: Docker Hub rate limit"),
			operation:    "docker pull",
			wantContains: []string{"docker pull", "Suggestions:", "rate limit", "wait 15 minutes"},
		},
		{
			name:         "COPY failed includes verification hint",
			err:          errors.New("COPY failed: stat /files: no such file"),
			operation:    "docker build",
			wantContains: []string{"docker build", "Suggestions:", "COPY failed", "verify the server build directory"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainerError(tt.err, tt.operation)

			if tt.wantNil {
				if got != nil {
					t.Errorf("ContainerError() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("ContainerError() returned nil, want error")
			}

			errMsg := got.Error()

			if tt.wantNotHints {
				if strings.Contains(errMsg, "Suggestions:") {
					t.Errorf("ContainerError() error should not contain hints, got: %s", errMsg)
				}
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(errMsg, want) {
					t.Errorf("ContainerError() error = %q, want to contain %q", errMsg, want)
				}
			}
		})
	}
}
