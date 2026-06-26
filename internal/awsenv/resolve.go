package awsenv

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/jpvelasco/ludus/internal/config"
)

// PlaceholderAccountID is returned under dry-run when no account ID is
// configured, so a representative URI can be printed without calling AWS.
const PlaceholderAccountID = "000000000000"

// placeholderRegion is used under dry-run when no region is configured, keeping
// dry-run network-free (no IMDS lookup).
const placeholderRegion = "us-east-1"

// IdentityAPI is the subset of the STS client used to resolve the account ID.
// The real *sts.Client satisfies it; tests inject a fake.
type IdentityAPI interface {
	GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// Requirements declares which fields a caller needs resolved. A target that
// needs no account (e.g. anywhere) leaves Account false so no STS call is made.
type Requirements struct {
	Account bool
	Region  bool
}

// Resolver resolves the AWS account ID and region from config, the SDK chain,
// STS, and IMDS. Construct with NewResolver.
type Resolver struct {
	dryRun bool

	// loadConfig loads the AWS SDK config for region; useIMDS enables EC2 IMDS
	// region resolution when region is empty. Overridable in tests.
	loadConfig func(ctx context.Context, region string, useIMDS bool) (aws.Config, error)
	// newIdentityClient builds an STS client from config. Overridable in tests.
	newIdentityClient func(aws.Config) IdentityAPI

	cached *Env
}

// NewResolver returns a Resolver wired to the real AWS SDK. Pass the process
// dry-run flag so resolution stays side-effect-free under --dry-run.
func NewResolver(dryRun bool) *Resolver {
	return &Resolver{
		dryRun:            dryRun,
		loadConfig:        defaultLoadConfig,
		newIdentityClient: func(c aws.Config) IdentityAPI { return sts.NewFromConfig(c) },
	}
}

func defaultLoadConfig(ctx context.Context, region string, useIMDS bool) (aws.Config, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	switch {
	case region != "":
		opts = append(opts, awsconfig.WithRegion(region))
	case useIMDS:
		opts = append(opts, awsconfig.WithEC2IMDSRegion())
	}
	return awsconfig.LoadDefaultConfig(ctx, opts...)
}

// Resolve resolves the requested fields and returns them plus the loaded SDK
// config. Results are memoized for the resolver's lifetime.
func (r *Resolver) Resolve(ctx context.Context, cfg *config.Config, req Requirements) (Env, error) {
	if r.cached != nil {
		return *r.cached, nil
	}

	region, awsCfg, err := r.resolveRegion(ctx, cfg)
	if err != nil {
		return Env{}, err
	}

	env := Env{Region: region, AWSConfig: awsCfg}

	if req.Account {
		account, err := r.resolveAccountID(ctx, cfg, awsCfg)
		if err != nil {
			return Env{}, err
		}
		env.AccountID = account
	}

	r.cached = &env
	return env, nil
}

// ResolveRegion resolves only the region.
func (r *Resolver) ResolveRegion(ctx context.Context, cfg *config.Config) (string, error) {
	env, err := r.Resolve(ctx, cfg, Requirements{Region: true})
	return env.Region, err
}

// ResolveAccountID resolves only the account ID. If the account ID is already
// configured, it is returned directly without requiring region resolution or
// an STS call.
func (r *Resolver) ResolveAccountID(ctx context.Context, cfg *config.Config) (string, error) {
	if id := strings.TrimSpace(cfg.AWS.AccountID); id != "" {
		return id, nil
	}
	if r.dryRun {
		return PlaceholderAccountID, nil
	}
	env, err := r.Resolve(ctx, cfg, Requirements{Account: true, Region: true})
	return env.AccountID, err
}

func (r *Resolver) resolveRegion(ctx context.Context, cfg *config.Config) (string, aws.Config, error) {
	useIMDS := !r.dryRun
	awsCfg, err := r.loadConfig(ctx, cfg.AWS.Region, useIMDS)
	if err != nil {
		return "", aws.Config{}, fmt.Errorf("loading AWS config: %w", err)
	}
	region := awsCfg.Region
	if strings.TrimSpace(region) == "" {
		if r.dryRun {
			region = placeholderRegion
			awsCfg.Region = region
		} else {
			return "", aws.Config{}, fmt.Errorf("aws.region is required or could not be resolved (set aws.region in ludus.yaml, or AWS_REGION / AWS_DEFAULT_REGION)")
		}
	}
	return region, awsCfg, nil
}

func (r *Resolver) resolveAccountID(ctx context.Context, cfg *config.Config, awsCfg aws.Config) (string, error) {
	if id := strings.TrimSpace(cfg.AWS.AccountID); id != "" {
		return id, nil
	}
	if r.dryRun {
		return PlaceholderAccountID, nil
	}
	id, err := AccountID(ctx, r.newIdentityClient(awsCfg))
	if err != nil {
		return "", fmt.Errorf("aws.accountId is required or could not be resolved: %w", err)
	}
	return id, nil
}

// AccountID calls STS GetCallerIdentity and returns the account ID. It is the
// single STS consolidation point used by the resolver and by deployers that
// already hold an STS client.
func AccountID(ctx context.Context, client IdentityAPI) (string, error) {
	out, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("calling sts get-caller-identity: %w", err)
	}
	id := strings.TrimSpace(aws.ToString(out.Account))
	if id == "" {
		return "", fmt.Errorf("sts get-caller-identity returned an empty account ID")
	}
	return id, nil
}
