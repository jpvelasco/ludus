// Package awsenv centralizes resolution of the AWS account ID and region and
// construction of ECR URIs, so no command duplicates this logic (issue #367).
package awsenv

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// Env holds a resolved AWS environment: account ID, region, and the loaded SDK
// config (so callers reuse one config instead of re-loading).
type Env struct {
	AccountID string
	Region    string
	AWSConfig aws.Config
}

// RegistryURI returns the ECR registry endpoint:
// <account>.dkr.ecr.<region>.amazonaws.com.
func RegistryURI(env Env) (string, error) {
	if err := requireEnv(env); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", env.AccountID, env.Region), nil
}

// RepositoryURI returns the registry endpoint plus repository, without a tag.
func RepositoryURI(env Env, repo string) (string, error) {
	reg, err := RegistryURI(env)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(repo) == "" {
		return "", fmt.Errorf("cannot build ECR repository URI: repository name is empty (set aws.ecrRepository in ludus.yaml)")
	}
	return reg + "/" + repo, nil
}

// ImageURI returns the fully qualified, tagged ECR image URI.
func ImageURI(env Env, repo, tag string) (string, error) {
	repoURI, err := RepositoryURI(env, repo)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(tag) == "" {
		return "", fmt.Errorf("cannot build ECR image URI: image tag is empty")
	}
	return repoURI + ":" + tag, nil
}

// requireEnv validates the account ID and region are present (not blank), so no
// URI builder can emit a string with an empty segment.
func requireEnv(env Env) error {
	if strings.TrimSpace(env.AccountID) == "" {
		return fmt.Errorf("aws.accountId is required or could not be resolved (set aws.accountId in ludus.yaml or ensure AWS credentials are valid)")
	}
	if strings.TrimSpace(env.Region) == "" {
		return fmt.Errorf("aws.region is required or could not be resolved (set aws.region in ludus.yaml, or AWS_REGION / AWS_DEFAULT_REGION)")
	}
	return nil
}
