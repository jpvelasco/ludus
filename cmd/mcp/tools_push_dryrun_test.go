package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

// forceNoAWSCredentials blanks every AWS configuration source (env vars and
// config files) so the SDK credential/region chain resolves nothing, making
// any live STS identity call fail deterministically and network-free. A
// dry-run preview must not need credentials at all (#727).
func forceNoAWSCredentials(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_PROFILE",
		"AWS_REGION",
		"AWS_DEFAULT_REGION",
		"AWS_WEB_IDENTITY_TOKEN_FILE",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_CONFIG_FILE", empty)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", empty)
}

// TestHandleContainerPushDryRunNeedsNoCredentials pins #727: a per-request
// dry_run=true must reach the AWS resolver, not just the subprocess runner.
// With no credentials anywhere, the push previews with placeholder identity
// instead of failing live STS resolution.
func TestHandleContainerPushDryRunNeedsNoCredentials(t *testing.T) {
	forceNoAWSCredentials(t)
	t.Chdir(t.TempDir())
	cfg := &config.Config{
		AWS:       config.AWSConfig{ECRRepository: "ludus-server"},
		Container: config.ContainerConfig{ImageName: "ludus-server", Tag: "v1"},
	}
	// The global flag is unset: only the schema dry_run input previews.
	globals.SetGlobals(t, cfg)

	result, _, err := handleContainerPush(context.Background(), nil, containerPushInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleContainerPush() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handleContainerPush(dry-run) failed without credentials: %s", toolResultText(t, result))
	}
	if text := toolResultText(t, result); !strings.Contains(text, `"success": true`) {
		t.Errorf("result = %q, want a successful dry-run push preview", text)
	}
}

// TestHandleEnginePushDryRunNeedsNoCredentials pins #727 for the engine push
// handler: the resolver receives the effective dry-run value there too.
func TestHandleEnginePushDryRunNeedsNoCredentials(t *testing.T) {
	forceNoAWSCredentials(t)
	t.Chdir(t.TempDir())
	cfg := &config.Config{
		Engine: config.EngineConfig{DockerImage: "ludus-engine:5.7.3"},
	}
	globals.SetGlobals(t, cfg)

	result, _, err := handleEnginePush(context.Background(), nil, enginePushInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleEnginePush() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("handleEnginePush(dry-run) failed without credentials: %s", toolResultText(t, result))
	}
	if text := toolResultText(t, result); !strings.Contains(text, `"success": true`) {
		t.Errorf("result = %q, want a successful dry-run push preview", text)
	}
}
