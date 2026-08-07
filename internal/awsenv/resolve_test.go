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
	tests := []resolveCase{
		{
			name:        "explicit config wins, no STS",
			region:      "us-west-2",
			cfgAccount:  "123456789012",
			cfgRegion:   "us-west-2",
			req:         Requirements{Account: true, Region: true},
			id:          &fakeIdentity{account: "999999999999"},
			wantAccount: "123456789012",
			wantRegion:  "us-west-2",
		},
		{
			name:        "account falls back to STS",
			region:      "eu-west-1",
			req:         Requirements{Account: true, Region: true},
			id:          &fakeIdentity{account: "555555555555"},
			wantAccount: "555555555555",
			wantRegion:  "eu-west-1",
			wantSTSCall: true,
		},
		{
			name:       "region-only requirement skips STS",
			region:     "us-east-2",
			req:        Requirements{Region: true},
			id:         &fakeIdentity{account: "x"},
			wantRegion: "us-east-2",
		},
		{
			name:        "dry-run returns placeholders without STS",
			dryRun:      true,
			req:         Requirements{Account: true, Region: true},
			id:          &fakeIdentity{err: errors.New("no creds")},
			wantAccount: PlaceholderAccountID,
			wantRegion:  placeholderRegion,
		},
		{
			name:    "account unresolved yields field error",
			region:  "us-west-2",
			req:     Requirements{Account: true},
			id:      &fakeIdentity{err: errors.New("no creds")},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertResolveCase(t, tt)
		})
	}
}

type resolveCase struct {
	name        string
	dryRun      bool
	region      string
	cfgAccount  string
	cfgRegion   string
	req         Requirements
	id          *fakeIdentity
	wantAccount string
	wantRegion  string
	wantErr     bool
	wantSTSCall bool
}

func assertResolveCase(t *testing.T, tt resolveCase) {
	t.Helper()
	r := newTestResolver(tt.dryRun, tt.region, tt.id)
	cfg := &config.Config{}
	cfg.AWS.AccountID = tt.cfgAccount
	cfg.AWS.Region = tt.cfgRegion

	env, err := r.Resolve(context.Background(), cfg, tt.req)
	if err != nil {
		if tt.wantErr {
			return
		}
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if tt.wantErr {
		t.Fatal("expected error, got nil")
	}
	if env.AccountID != tt.wantAccount {
		t.Errorf("account = %q, want %q", env.AccountID, tt.wantAccount)
	}
	if env.Region != tt.wantRegion {
		t.Errorf("region = %q, want %q", env.Region, tt.wantRegion)
	}
	if (tt.id.calls == 0) != (tt.wantSTSCall == false) {
		t.Errorf("STS called %d times, wantSTSCall=%v", tt.id.calls, tt.wantSTSCall)
	}
}

func TestResolve_Memoizes(t *testing.T) {
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
}

func TestResolveRegionWrapper(t *testing.T) {
	id := &fakeIdentity{account: "000"}
	r := newTestResolver(false, "ap-southeast-1", id)
	cfg := &config.Config{}

	region, err := r.ResolveRegion(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if region != "ap-southeast-1" {
		t.Errorf("got region %q, want 'ap-southeast-1'", region)
	}
	if id.calls != 0 {
		t.Errorf("STS called %d times, want 0 (region-only)", id.calls)
	}
}

func TestResolveAccountIDWrapper(t *testing.T) {
	id := &fakeIdentity{account: "222222222222"}
	r := newTestResolver(false, "us-east-1", id)
	cfg := &config.Config{}

	account, err := r.ResolveAccountID(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if account != "222222222222" {
		t.Errorf("got account %q, want '222222222222'", account)
	}
}

func TestResolve_LoadConfigError(t *testing.T) {
	r := newTestResolver(false, "us-west-2", &fakeIdentity{})
	r.loadConfig = func(_ context.Context, _ string, _ bool) (aws.Config, error) {
		return aws.Config{}, errors.New("no config")
	}
	cfg := &config.Config{}

	_, err := r.Resolve(context.Background(), cfg, Requirements{Region: true})
	if err == nil {
		t.Fatal("expected error from failed config load, got nil")
	}
}

func TestResolve_EmptyRegionNonDryRun(t *testing.T) {
	r := newTestResolver(false, "", &fakeIdentity{})
	cfg := &config.Config{}

	_, err := r.Resolve(context.Background(), cfg, Requirements{Region: true})
	if err == nil {
		t.Fatal("expected error for unresolved region without dry-run, got nil")
	}
}

func TestAccountID(t *testing.T) {
	t.Run("returns account", func(t *testing.T) {
		id, err := AccountID(context.Background(), &fakeIdentity{account: "123456789012"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "123456789012" {
			t.Errorf("got %q, want '123456789012'", id)
		}
	})

	t.Run("surfaces sts error", func(t *testing.T) {
		_, err := AccountID(context.Background(), &fakeIdentity{err: errors.New("denied")})
		if err == nil {
			t.Fatal("expected error from sts, got nil")
		}
	})

	t.Run("rejects empty account", func(t *testing.T) {
		_, err := AccountID(context.Background(), &fakeIdentity{account: "   "})
		if err == nil {
			t.Fatal("expected error for empty account ID, got nil")
		}
	})
}

func TestNewResolver_DefaultWiring(t *testing.T) {
	// The real resolver's default closures must be wired: loadConfig resolves
	// a region from the SDK chain, and newIdentityClient builds an STS client.
	r := NewResolver(true)
	if r.loadConfig == nil {
		t.Fatal("NewResolver left loadConfig nil")
	}
	if r.newIdentityClient == nil {
		t.Fatal("NewResolver left newIdentityClient nil")
	}

	cfg, err := r.loadConfig(context.Background(), "us-east-1", false)
	if err != nil {
		t.Fatalf("defaultLoadConfig failed: %v", err)
	}
	if cfg.Region != "us-east-1" {
		t.Errorf("loadConfig region = %q, want 'us-east-1'", cfg.Region)
	}

	if id := r.newIdentityClient(cfg); id == nil {
		t.Fatal("newIdentityClient returned nil STS client")
	}
}
