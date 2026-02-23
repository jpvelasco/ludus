package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateWorkflow(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		content := GenerateWorkflow(WorkflowOptions{
			RunnerLabels: []string{"self-hosted", "linux", "x64"},
		})

		// Check essential elements
		checks := []string{
			"name: Ludus UE5 Pipeline",
			"workflow_dispatch:",
			"skip-engine:",
			"default: true",
			"runs-on: [self-hosted, linux, x64]",
			"timeout-minutes: 480",
			"actions/checkout@v4",
			"actions/setup-go@v6",
			"go build -o ludus -v",
			"./ludus init --verbose",
			"./ludus run --verbose",
			"./ludus status",
			"cancel-in-progress: false",
			"# push:",
			"# pull_request:",
		}
		for _, check := range checks {
			if !strings.Contains(content, check) {
				t.Errorf("expected workflow to contain %q", check)
			}
		}
	})

	t.Run("enable push trigger", func(t *testing.T) {
		content := GenerateWorkflow(WorkflowOptions{
			RunnerLabels: []string{"self-hosted", "linux", "x64"},
			EnablePush:   true,
		})

		if !strings.Contains(content, "  push:\n    branches: [main]") {
			t.Error("expected uncommented push trigger")
		}
		// PR should still be commented
		if !strings.Contains(content, "# pull_request:") {
			t.Error("expected PR trigger to remain commented")
		}
	})

	t.Run("enable PR trigger", func(t *testing.T) {
		content := GenerateWorkflow(WorkflowOptions{
			RunnerLabels: []string{"self-hosted", "linux", "x64"},
			EnablePR:     true,
		})

		if !strings.Contains(content, "  pull_request:\n    branches: [main]") {
			t.Error("expected uncommented PR trigger")
		}
		// Push should still be commented
		if !strings.Contains(content, "# push:") {
			t.Error("expected push trigger to remain commented")
		}
	})

	t.Run("custom labels", func(t *testing.T) {
		content := GenerateWorkflow(WorkflowOptions{
			RunnerLabels: []string{"self-hosted", "linux", "arm64", "ue5"},
		})

		if !strings.Contains(content, "runs-on: [self-hosted, linux, arm64, ue5]") {
			t.Error("expected custom labels in runs-on")
		}
	})
}

func TestWriteWorkflow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "workflow.yml")

	err := WriteWorkflow(path, "test content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != "test content" {
		t.Errorf("expected 'test content', got %q", string(data))
	}
}

func TestFormatLabels(t *testing.T) {
	tests := []struct {
		labels []string
		want   string
	}{
		{[]string{"self-hosted"}, "[self-hosted]"},
		{[]string{"self-hosted", "linux", "x64"}, "[self-hosted, linux, x64]"},
	}
	for _, tt := range tests {
		got := formatLabels(tt.labels)
		if got != tt.want {
			t.Errorf("formatLabels(%v) = %q, want %q", tt.labels, got, tt.want)
		}
	}
}
