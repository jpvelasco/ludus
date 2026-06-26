package globals

import (
	"context"

	"github.com/jpvelasco/ludus/internal/awsenv"
	"github.com/jpvelasco/ludus/internal/config"
)

// ResolveAWSAccountID returns the AWS account ID from the given value, or
// auto-detects it via STS when empty. Delegates to internal/awsenv.
// The region parameter is used when auto-detecting the account ID via STS,
// since the AWS SDK requires a region to make the call.
func ResolveAWSAccountID(ctx context.Context, accountID, region string) (string, error) {
	cfg := &config.Config{}
	cfg.AWS.AccountID = accountID
	cfg.AWS.Region = region
	return awsenv.NewResolver(DryRun).ResolveAccountID(ctx, cfg)
}

// ResolveAWSRegion returns the region from the given value, or resolves it via
// the AWS SDK chain / IMDS when empty. Delegates to internal/awsenv.
func ResolveAWSRegion(region string) (string, error) {
	cfg := &config.Config{}
	cfg.AWS.Region = region
	return awsenv.NewResolver(DryRun).ResolveRegion(context.Background(), cfg)
}
