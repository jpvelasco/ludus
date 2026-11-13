package ecr

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestRepositoryInspectionClassification(t *testing.T) {
	tests := []struct {
		name, message, want string
		code                int
		create              bool
	}{
		{"exists", "", "", 0, false},
		{"denied", "AccessDeniedException", "ecr:DescribeRepositories", 255, false},
		{"network", "connection failed", "describing ECR repository", 255, false},
		{"missing", "RepositoryNotFoundException", "creating ECR repository", 255, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTool(t, "aws", testsupport.ToolBehavior{ExitCode: tt.code, Stderr: tt.message})
			var stdout, stderr bytes.Buffer
			r := &runner.Runner{Verbose: true, Stdout: &stdout, Stderr: &stderr}
			err := ensureECRRepository(context.Background(), r, PushOptions{ECRRepository: "example", AWSRegion: "us-east-1"})
			if (err != nil) != (tt.want != "") {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil && !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %s", err, tt.want)
			}
			if strings.Contains(stdout.String(), "create-repository") != tt.create {
				t.Errorf("unexpected create attempt: %s", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("raw CLI error streamed: %s", stderr.String())
			}
		})
	}
}

func TestCreateRepositoryConfiguredTags(t *testing.T) {
	want := map[string]string{"managedby": "example team", "Project": `a,b "quoted"`, "Environment": "test"}
	var output bytes.Buffer
	r := &runner.Runner{Stdout: &output, Stderr: &bytes.Buffer{}, DryRun: true}
	if err := createECRRepository(context.Background(), r, PushOptions{Tags: want}); err != nil {
		t.Fatal(err)
	}
	_, encoded, ok := strings.Cut(output.String(), "--tags ")
	if !ok {
		t.Fatal("create command missing tags")
	}
	var list []struct{ Key, Value string }
	if err := json.Unmarshal([]byte(encoded), &list); err != nil {
		t.Fatalf("invalid JSON tags argument: %v", err)
	}
	got := make(map[string]string)
	for _, tag := range list {
		got[tag.Key] = tag.Value
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tags = %v, want %v", got, want)
	}
}

func TestCreateRepositoryDenied(t *testing.T) {
	testsupport.FakeTool(t, "aws", testsupport.ToolBehavior{ExitCode: 255, Stderr: "AccessDeniedException"})
	err := createECRRepository(context.Background(), runner.NewRunner(false, false), PushOptions{ECRRepository: "example"})
	if err == nil || !strings.Contains(err.Error(), "ecr:CreateRepository") {
		t.Fatalf("expected create permission remedy, got %v", err)
	}
}
