package ecr

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
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
	if !strings.Contains(err.Error(), "account ID") {
		t.Errorf("error should mention account ID, got: %v", err)
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
