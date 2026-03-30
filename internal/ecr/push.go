package ecr

import (
	"context"
	"fmt"
	"strings"

	"github.com/devrecon/ludus/internal/retry"
	"github.com/devrecon/ludus/internal/runner"
)

// PushOptions configures an ECR push operation.
type PushOptions struct {
	// ECRRepository is the ECR repository name (e.g. "ludus-server").
	ECRRepository string
	// AWSRegion is the AWS region (e.g. "us-east-1").
	AWSRegion string
	// AWSAccountID is the AWS account ID.
	AWSAccountID string
	// ImageTag is the remote image tag in ECR (e.g. "latest").
	ImageTag string
}

// Push authenticates with ECR, ensures the repository exists, tags the local
// image, and pushes it to ECR. All network operations are retried with
// exponential backoff.
//
// localTag is the existing Docker image tag (e.g. "ludus-server:latest").
func Push(ctx context.Context, r *runner.Runner, localTag string, opts PushOptions) error {
	if opts.AWSAccountID == "" {
		return fmt.Errorf("AWS account ID not configured (set aws.accountId in ludus.yaml)")
	}

	ecrURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s",
		opts.AWSAccountID, opts.AWSRegion, opts.ECRRepository)

	// Ensure ECR repository exists (create if missing).
	if err := r.RunQuiet(ctx, "aws", "ecr", "describe-repositories",
		"--repository-names", opts.ECRRepository,
		"--region", opts.AWSRegion); err != nil {
		fmt.Printf("  ECR repository %q not found, creating...\n", opts.ECRRepository)
		if err := r.RunQuiet(ctx, "aws", "ecr", "create-repository",
			"--repository-name", opts.ECRRepository,
			"--region", opts.AWSRegion,
			"--image-scanning-configuration", "scanOnPush=true",
			"--tags", "Key=ManagedBy,Value=ludus"); err != nil {
			return fmt.Errorf("creating ECR repository: %w", err)
		}
	}

	// Authenticate with ECR — get password then pipe to docker login.
	// Both steps are retried since ECR auth tokens can fail on transient errors.
	loginURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", opts.AWSAccountID, opts.AWSRegion)
	retryCfg := retry.Default()
	var password []byte
	if err := retry.Do(ctx, retryCfg, func() error {
		var err error
		password, err = r.RunOutput(ctx, "aws", "ecr", "get-login-password", "--region", opts.AWSRegion)
		return err
	}); err != nil {
		return fmt.Errorf("getting ECR password: %w", err)
	}
	if err := retry.Do(ctx, retryCfg, func() error {
		return r.RunQuietWithStdin(ctx, strings.NewReader(strings.TrimSpace(string(password))),
			"docker", "login", "--username", "AWS", "--password-stdin", loginURI)
	}); err != nil {
		return fmt.Errorf("ECR login failed: %w", err)
	}
	fmt.Println("  ECR login succeeded")

	// Tag and push.
	remoteTag := fmt.Sprintf("%s:%s", ecrURI, opts.ImageTag)
	if err := r.RunQuiet(ctx, "docker", "tag", localTag, remoteTag); err != nil {
		return fmt.Errorf("docker tag failed: %w", err)
	}
	if err := retry.Do(ctx, retryCfg, func() error {
		return r.Run(ctx, "docker", "push", remoteTag)
	}); err != nil {
		return fmt.Errorf("docker push failed: %w", err)
	}

	return nil
}
