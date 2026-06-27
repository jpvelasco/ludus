package ecr

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

func TestPush_MissingAccountID(t *testing.T) {
	r := runner.NewRunner(false, true)
	err := Push(context.Background(), r, "test:latest", PushOptions{
		ECRRepository: "test",
		AWSRegion:     "us-east-1",
		AWSAccountID:  "",
		ImageTag:      "latest",
	})
	if err == nil {
		t.Fatal("expected error for missing account ID")
	}
	if !strings.Contains(err.Error(), "aws.accountId is required") {
		t.Errorf("error should mention aws.accountId, got: %v", err)
	}
}

func TestPush_MissingRegion(t *testing.T) {
	r := runner.NewRunner(false, true)
	err := Push(context.Background(), r, "test:latest", PushOptions{
		ECRRepository: "test",
		AWSRegion:     "",
		AWSAccountID:  "123456789012",
		ImageTag:      "latest",
	})
	if err == nil {
		t.Fatal("expected error for missing region")
	}
	if !strings.Contains(err.Error(), "aws.region is required") {
		t.Errorf("error should mention aws.region, got: %v", err)
	}
}

func TestPush_MissingRepository(t *testing.T) {
	r := runner.NewRunner(false, true)
	err := Push(context.Background(), r, "test:latest", PushOptions{
		ECRRepository: "",
		AWSRegion:     "us-east-1",
		AWSAccountID:  "123456789012",
		ImageTag:      "latest",
	})
	if err == nil {
		t.Fatal("expected error for missing repository")
	}
	if !strings.Contains(err.Error(), "repository name is empty") {
		t.Errorf("error should mention repository name, got: %v", err)
	}
}

func TestPush_DefaultsImageTag(t *testing.T) {
	var stdout bytes.Buffer
	r := &runner.Runner{
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
		Verbose: true,
		DryRun:  true,
	}
	err := Push(context.Background(), r, "test:latest", PushOptions{
		ECRRepository: "test",
		AWSRegion:     "us-east-1",
		AWSAccountID:  "123456789012",
		ImageTag:      "",
	})
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	if !strings.Contains(stdout.String(), ":latest") {
		t.Error("empty ImageTag should default to 'latest'")
	}
}

func TestPush_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := &runner.Runner{
		Stdout:  &stdout,
		Stderr:  &stderr,
		Verbose: true,
		DryRun:  true,
	}
	err := Push(context.Background(), r, "ludus-server:latest", PushOptions{
		ECRRepository: "ludus-server",
		AWSRegion:     "us-east-1",
		AWSAccountID:  "123456789012",
		ImageTag:      "latest",
	})
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "aws ecr describe-repositories") {
		t.Error("dry-run output should contain describe-repositories command")
	}
	if !strings.Contains(output, "aws ecr get-login-password") {
		t.Error("dry-run output should contain get-login-password command")
	}
	if !strings.Contains(output, "docker tag") {
		t.Error("dry-run output should contain docker tag command")
	}
	if !strings.Contains(output, "docker push") {
		t.Error("dry-run output should contain docker push command")
	}
}

func TestIsAccessDenied(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{"AccessDeniedException", "operation error ECR: CreateRepository, AccessDeniedException: ...", true},
		{"not authorized", "User ... is not authorized to perform: ecr:CreateRepository", true},
		{"unrelated error", "RepositoryAlreadyExists", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAccessDenied(fmt.Errorf("%s", tt.msg)); got != tt.want {
				t.Errorf("isAccessDenied(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}
