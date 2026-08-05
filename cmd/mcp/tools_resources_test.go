package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

// TestHandleResourcesLoadAWSConfigFailure drives handleResources through its
// LoadAWSConfig error branch (tools_resources.go:32-35). A bogus AWS profile
// makes awsconfig.LoadDefaultConfig fail deterministically and without network,
// covering region defaulting (input empty -> config region) and the explicit
// region override path (input set -> used verbatim).
func TestHandleResourcesLoadAWSConfigFailure(t *testing.T) {
	t.Setenv("AWS_PROFILE", "ludus-does-not-exist-anywhere")
	t.Setenv("AWS_SDK_LOAD_CONFIG", "1")

	tests := []struct {
		name  string
		input resourcesInput
	}{
		{name: "region defaults from config", input: resourcesInput{}},
		{name: "explicit region override", input: resourcesInput{Region: "eu-central-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCfg := globals.Cfg
			t.Cleanup(func() { globals.Cfg = origCfg })
			globals.Cfg = &config.Config{AWS: config.AWSConfig{Region: "us-west-2"}}

			result, _, err := handleResources(context.Background(), nil, tt.input)
			if err != nil {
				t.Fatalf("handleResources() error = %v", err)
			}
			if !result.IsError {
				t.Fatal("handleResources() should return an error result")
			}
			if text := toolResultText(t, result); !strings.Contains(text, "could not load AWS config") {
				t.Errorf("result = %q, want AWS config load failure", text)
			}
		})
	}
}
