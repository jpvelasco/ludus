package globals

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ResolveAWSAccountID returns the AWS account ID from cfg, or auto-detects it
// via aws sts get-caller-identity if the config value is empty.
func ResolveAWSAccountID(ctx context.Context, accountID string) (string, error) {
	if accountID != "" {
		return accountID, nil
	}
	out, err := exec.CommandContext(ctx, "aws", "sts", "get-caller-identity", "--output", "json").Output()
	if err != nil {
		return "", fmt.Errorf("aws.accountId not configured and auto-detection failed: %w\n  Set aws.accountId in ludus.yaml or ensure AWS credentials are valid", err)
	}
	var identity struct {
		Account string `json:"Account"`
	}
	if err := json.Unmarshal(out, &identity); err != nil || strings.TrimSpace(identity.Account) == "" {
		return "", fmt.Errorf("aws.accountId not configured and could not be parsed from aws sts get-caller-identity output")
	}
	return strings.TrimSpace(identity.Account), nil
}

// ResolveAWSRegion returns the AWS region from cfg, or falls back to
// AWS_DEFAULT_REGION / AWS_REGION environment variables.
func ResolveAWSRegion(region string) (string, error) {
	if region != "" {
		return region, nil
	}
	for _, env := range []string{"AWS_DEFAULT_REGION", "AWS_REGION"} {
		if v := os.Getenv(env); v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("aws.region not configured (set aws.region in ludus.yaml, or AWS_DEFAULT_REGION / AWS_REGION env var)")
}
