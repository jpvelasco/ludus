package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

// forceNoAWS blanks every AWS configuration source (env vars and config files)
// so the SDK credential/region chain resolves nothing, making detectAWSAccountID
// deterministic and network-free.
func forceNoAWS(t *testing.T) {
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

func TestDetectAWSAccountIDNoCredentials(t *testing.T) {
	forceNoAWS(t)
	if got := detectAWSAccountID(); got != "" {
		t.Errorf("detectAWSAccountID() = %q, want empty", got)
	}
}

func TestPromptAWSDefaultNoDetection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		wantR string
		wantA string
	}{
		{name: "typed values", input: "eu-west-1\n123456789012\n", wantR: "eu-west-1", wantA: "123456789012"},
		{name: "defaults", input: "\n\n", wantR: "us-east-1", wantA: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forceNoAWS(t)
			withScannerInput(t, tt.input)
			region, account := promptAWSDefault("us-east-1", nil)
			if region != tt.wantR || account != tt.wantA {
				t.Errorf("promptAWSDefault() = %q, %q; want %q, %q", region, account, tt.wantR, tt.wantA)
			}
		})
	}
}

func TestPromptAWSDefaultExistingAccount(t *testing.T) {
	forceNoAWS(t)
	existing := &config.Config{}
	existing.AWS.AccountID = "111122223333"
	withScannerInput(t, "\n\n")
	region, account := promptAWSDefault("us-east-1", existing)
	if region != "us-east-1" || account != "111122223333" {
		t.Errorf("promptAWSDefault() = %q, %q; want us-east-1, 111122223333", region, account)
	}
}
