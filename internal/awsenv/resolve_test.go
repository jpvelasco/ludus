package awsenv

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/jpvelasco/ludus/internal/config"
)

type fakeIdentity struct {
	account string
	err     error
	calls   int
}

func (f *fakeIdentity) GetCallerIdentity(_ context.Context, _ *sts.GetCallerIdentityInput, _ ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &sts.GetCallerIdentityOutput{Account: aws.String(f.account)}, nil
}

// newTestResolver builds a resolver with injected, network-free seams.
func newTestResolver(dryRun bool, region string, id *fakeIdentity) *Resolver {
	r := NewResolver(dryRun)
	r.loadConfig = func(_ context.Context, reg string, _ bool) (aws.Config, error) {
		if reg == "" {
			reg = region // simulate SDK/IMDS-resolved region
		}
		return aws.Config{Region: reg}, nil
	}
	r.newIdentityClient = func(aws.Config) IdentityAPI { return id }
	return r
}

func TestResolve(t *testing.T) {
	t.Run("explicit config wins, no STS", func(t *testing.T) {
		id := &fakeIdentity{account: "999999999999"}
		r := newTestResolver(false, "", id)
		cfg := &config.Config{}
		cfg.AWS.AccountID = "123456789012"
		cfg.AWS.Region = "us-west-2"

		env, err := r.Resolve(context.Background(), cfg, Requirements{Account: true, Region: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env.AccountID != "123456789012" || env.Region != "us-west-2" {
			t.Errorf("got %+v", env)
		}
		if id.calls != 0 {
			t.Errorf("STS called %d times, want 0", id.calls)
		}
	})

	t.Run("account falls back to STS", func(t *testing.T) {
		id := &fakeIdentity{account: "555555555555"}
		r := newTestResolver(false, "eu-west-1", id)
		cfg := &config.Config{}

		env, err := r.Resolve(context.Background(), cfg, Requirements{Account: true, Region: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env.AccountID != "555555555555" {
			t.Errorf("got account %q", env.AccountID)
		}
		if env.Region != "eu-west-1" {
			t.Errorf("got region %q", env.Region)
		}
	})

	t.Run("region-only requirement skips STS", func(t *testing.T) {
		id := &fakeIdentity{account: "x"}
		r := newTestResolver(false, "us-east-2", id)
		cfg := &config.Config{}

		env, err := r.Resolve(context.Background(), cfg, Requirements{Region: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env.Region != "us-east-2" {
			t.Errorf("got region %q", env.Region)
		}
		if id.calls != 0 {
			t.Errorf("STS called %d times, want 0", id.calls)
		}
	})

	t.Run("dry-run returns placeholders without STS", func(t *testing.T) {
		id := &fakeIdentity{err: errors.New("no creds")}
		r := newTestResolver(true, "", id)
		cfg := &config.Config{}

		env, err := r.Resolve(context.Background(), cfg, Requirements{Account: true, Region: true})
		if err != nil {
			t.Fatalf("dry-run must not error: %v", err)
		}
		if env.AccountID != PlaceholderAccountID {
			t.Errorf("got account %q, want placeholder", env.AccountID)
		}
		if env.Region != placeholderRegion {
			t.Errorf("got region %q, want placeholder", env.Region)
		}
		if id.calls != 0 {
			t.Errorf("STS called %d times in dry-run, want 0", id.calls)
		}
	})

	t.Run("memoizes — STS called at most once", func(t *testing.T) {
		id := &fakeIdentity{account: "111111111111"}
		r := newTestResolver(false, "us-west-2", id)
		cfg := &config.Config{}

		for i := 0; i < 3; i++ {
			if _, err := r.Resolve(context.Background(), cfg, Requirements{Account: true, Region: true}); err != nil {
				t.Fatalf("call %d: %v", i, err)
			}
		}
		if id.calls != 1 {
			t.Errorf("STS called %d times, want 1", id.calls)
		}
	})

	t.Run("account unresolved yields field error", func(t *testing.T) {
		id := &fakeIdentity{err: errors.New("no creds")}
		r := newTestResolver(false, "us-west-2", id)
		cfg := &config.Config{}

		_, err := r.Resolve(context.Background(), cfg, Requirements{Account: true})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
