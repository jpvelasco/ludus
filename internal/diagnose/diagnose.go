// Package diagnose provides contextual error guidance for common failure modes.
// Each function inspects an error and returns an enhanced error with actionable
// fix suggestions, or the original error unchanged if no guidance is available.
package diagnose

import (
	"fmt"
	"strings"
)

// errorHint maps an error substring to user-facing guidance.
type errorHint struct {
	pattern string
	hint    string
}

// AWSError inspects an AWS API error and returns guidance.
func AWSError(err error, operation string) error {
	if err == nil {
		return nil
	}

	msg := err.Error()
	hints := matchHints(msg, awsHints)

	if len(hints) == 0 {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return wrapWithHints(err, operation, hints)
}

// DeployError inspects a deployment error and returns guidance.
func DeployError(err error, target string) error {
	if err == nil {
		return nil
	}

	msg := err.Error()
	hints := matchHints(msg, deployHints)
	hints = append(hints, matchHints(msg, awsHints)...)

	if len(hints) == 0 {
		return fmt.Errorf("deploy %s failed: %w", target, err)
	}

	return wrapWithHints(err, fmt.Sprintf("deploy %s failed", target), hints)
}

// ContainerError inspects a Docker/container error and returns guidance.
func ContainerError(err error, operation string) error {
	if err == nil {
		return nil
	}

	msg := err.Error()
	hints := matchHints(msg, containerHints)

	if len(hints) == 0 {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return wrapWithHints(err, operation, hints)
}

var awsHints = []errorHint{
	{"ExpiredTokenException", "AWS session expired; run 'aws sso login' or refresh credentials"},
	{"ExpiredToken", "AWS session expired; run 'aws sso login' or refresh credentials"},
	{"InvalidClientTokenId", "AWS access key is invalid; run 'aws configure' to set new credentials"},
	{"SignatureDoesNotMatch", "AWS secret key mismatch; run 'aws configure' to update credentials"},
	{"AccessDeniedException", "insufficient IAM permissions; ensure your role has GameLift, IAM, S3, and ECR access"},
	{"AccessDenied", "insufficient IAM permissions; check your IAM policy"},
	{"UnauthorizedAccess", "AWS authorization failed; verify your IAM role and permissions"},
	{"LimitExceededException", "AWS service limit reached; request a quota increase via AWS Service Quotas console"},
	{"ServiceQuotaExceededException", "AWS quota exceeded; request an increase via AWS Service Quotas console"},
	{"ResourceNotFoundException", "AWS resource not found; it may have been deleted or the region is wrong"},
	{"NoCredentialProviders", "no AWS credentials found; run 'aws configure' or set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY"},
	{"RequestExpired", "AWS request expired; check system clock or refresh credentials"},
}

var deployHints = []errorHint{
	{"fleet is in ERROR", "fleet creation failed; check AWS GameLift console for details, then destroy and redeploy"},
	{"timed out waiting for", "deployment timed out; this can happen with large builds — check the AWS console for fleet status"},
	{"InternalServiceError", "AWS internal error; wait a few minutes and retry"},
	{"ConflictException", "resource already exists; run 'ludus deploy destroy' first, then redeploy"},
	{"InvalidRequestException", "invalid fleet configuration; verify instance type and region support GameLift"},
	{"no non-loopback IPv4", "could not detect local IP; use --ip flag to specify your machine's IP address"},
}

var containerHints = []errorHint{
	{"no space left on device", "disk full; free space with 'docker system prune' or remove unused images"},
	{"Cannot connect to the Docker daemon", "Docker not running; start Docker Desktop or 'sudo systemctl start docker'"},
	{"denied: Your authorization token has expired", "ECR login expired; Ludus re-authenticates automatically — retry the command"},
	{"toomanyrequests", "Docker Hub rate limit; wait 15 minutes or use an authenticated pull"},
	{"error getting credentials", "Docker credential helper failed; check ~/.docker/config.json"},
	{"COPY failed", "Docker COPY failed; verify the server build directory exists and contains the expected files"},
}

// matchHints returns all matching hints for the given error message.
func matchHints(msg string, hints []errorHint) []string {
	var matched []string
	seen := make(map[string]bool)
	for _, h := range hints {
		if strings.Contains(msg, h.pattern) && !seen[h.hint] {
			matched = append(matched, h.hint)
			seen[h.hint] = true
		}
	}
	return matched
}

// wrapWithHints formats an error with actionable suggestions.
func wrapWithHints(err error, operation string, hints []string) error {
	var sb strings.Builder
	sb.WriteString(operation)
	sb.WriteString("\n\nSuggestions:")
	for _, h := range hints {
		sb.WriteString("\n  - ")
		sb.WriteString(h)
	}
	return fmt.Errorf("%s: %w", sb.String(), err)
}
