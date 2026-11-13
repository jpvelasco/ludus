package cleanup

import (
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/ec2fleet"
)

// DurableNames is exactly the named scope of purge, independent of inventory.
type DurableNames struct {
	ECRRepository string
	S3Bucket      string
}

// ResolveDurableNames shares configured/default names without resolving AWS
// identity. A missing account leaves only the default bucket unresolved.
func ResolveDurableNames(cfg *config.Config, accountID string) DurableNames {
	names := DurableNames{ECRRepository: cfg.AWS.ECRRepository, S3Bucket: cfg.EC2Fleet.S3Bucket}
	if names.ECRRepository == "" {
		names.ECRRepository = "ludus-server"
	}
	if names.S3Bucket == "" && accountID != "" {
		names.S3Bucket = ec2fleet.BuildBucketName(accountID)
	}
	return names
}

// PlanPurge describes the same two durable artifacts as live purge. It does
// not select engine repositories, other profiles, or state-recorded buckets.
func PlanPurge(cfg *config.Config) []deploy.DestroyItem {
	names := ResolveDurableNames(cfg, cfg.AWS.AccountID)
	repo := deploy.DestroyItem{
		Kind: "ecr-repository", Identifier: names.ECRRepository, Source: deploy.SourceConfig,
		Action: deploy.ActionDelete, Decision: deploy.DecisionPlanned,
		Reason: "Delete the server repository and all images, selected by name without an ownership tag guard.",
	}
	if cfg.AWS.ECRRepository == "" {
		repo.Source = deploy.SourceDefault
	}
	bucket := deploy.DestroyItem{
		Kind: "s3-bucket", Identifier: names.S3Bucket, Source: deploy.SourceConfig,
		Action: deploy.ActionDelete, Decision: deploy.DecisionPlanned,
		Reason: "Delete the bucket and its contents by name, without an ownership tag guard; object versions or locks may prevent completion.",
	}
	if cfg.EC2Fleet.S3Bucket == "" {
		bucket.Source = deploy.SourceDefault
	}
	if names.S3Bucket == "" {
		bucket.Decision, bucket.Source = deploy.DecisionUnresolved, deploy.SourceDiscovery
		bucket.Selector = "account-derived build bucket; resolve AWS account during execution"
	}
	return []deploy.DestroyItem{repo, bucket}
}
