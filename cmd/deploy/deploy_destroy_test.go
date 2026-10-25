package deploy

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/jpvelasco/ludus/internal/config"
)

// mockCleaner records the repository and bucket names the cleanup path hands to
// the AWS cleaner, without touching AWS.
type mockCleaner struct {
	ecrRepo  string
	bucket   string
	ecrCalls int
	s3Calls  int
	ecrErr   error
	s3Err    error
}

func (m *mockCleaner) DeleteECRRepository(_ context.Context, repoName string) error {
	m.ecrRepo = repoName
	m.ecrCalls++
	return m.ecrErr
}

func (m *mockCleaner) DeleteS3Bucket(_ context.Context, bucketName string) error {
	m.bucket = bucketName
	m.s3Calls++
	return m.s3Err
}

func TestBuildBucketName(t *testing.T) {
	if got := buildBucketName("123456789012"); got != "ludus-builds-123456789012" {
		t.Errorf("buildBucketName = %q, want ludus-builds-123456789012", got)
	}
}

// TestResolveBuildBucket pins the purge target: the configured
// ec2fleet.s3Bucket wins, otherwise the account-derived default; an
// unresolvable account yields no bucket (deletion skipped).
func TestResolveBuildBucket(t *testing.T) {
	tests := []struct {
		name      string
		s3Bucket  string
		accountID string
		want      string
	}{
		{"custom bucket wins", "example-custom-build-storage", "123456789012", "example-custom-build-storage"},
		{"default derives from account", "", "123456789012", "ludus-builds-123456789012"},
		{"no bucket without account", "", "", ""},
		{"custom bucket works without account", "example-custom-build-storage", "", "example-custom-build-storage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.EC2Fleet.S3Bucket = tt.s3Bucket
			if got := resolveBuildBucket(cfg, tt.accountID); got != tt.want {
				t.Errorf("resolveBuildBucket = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveDestroyScope(t *testing.T) {
	tests := []struct {
		name        string
		allTargets  bool
		purge       bool
		wantSweep   bool
		wantDurable bool
	}{
		{"default: ephemeral, active target only", false, false, false, false},
		{"all-targets: sweep, no durable", true, false, true, false},
		{"purge: durable, active target only", false, true, false, true},
		{"all-targets + purge: full wipe", true, true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDestroyScope(tt.allTargets, tt.purge)
			if got.sweep != tt.wantSweep || got.durable != tt.wantDurable {
				t.Errorf("resolveDestroyScope(%v,%v) = {sweep:%v durable:%v}, want {sweep:%v durable:%v}",
					tt.allTargets, tt.purge, got.sweep, got.durable, tt.wantSweep, tt.wantDurable)
			}
		})
	}
}

func TestConfirmPurge(t *testing.T) {
	items := []string{"ECR repository: lyra-server-x86 (and all images)"}

	tests := []struct {
		name  string
		input string
		skip  bool
		want  bool
	}{
		{"skip via --yes (no prompt read)", "", true, true},
		{"explicit y", "y\n", false, true},
		{"explicit yes", "yes\n", false, true},
		{"uppercase Y", "Y\n", false, true},
		{"explicit n", "n\n", false, false},
		{"empty defaults to no", "\n", false, false},
		{"garbage is no", "maybe\n", false, false},
		{"EOF is no", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			got := confirmPurge(&out, strings.NewReader(tt.input), items, tt.skip)
			if got != tt.want {
				t.Errorf("confirmPurge(input=%q, skip=%v) = %v, want %v", tt.input, tt.skip, got, tt.want)
			}
			// The destructive items must always be shown to the user, even when skipping.
			if !strings.Contains(out.String(), items[0]) {
				t.Errorf("confirmPurge should list the durable items; output:\n%s", out.String())
			}
		})
	}
}

func TestPurgeItems(t *testing.T) {
	t.Run("uses configured ECR repo and account", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.AWS.ECRRepository = "lyra-server-x86"
		cfg.AWS.AccountID = "123456789012"
		items := purgeItems(cfg)
		joined := strings.Join(items, "\n")
		if !strings.Contains(joined, "lyra-server-x86") {
			t.Errorf("purgeItems should name the ECR repo; got %v", items)
		}
		if !strings.Contains(joined, buildBucketName("123456789012")) {
			t.Errorf("purgeItems should name the S3 build bucket; got %v", items)
		}
	})

	t.Run("names the configured custom EC2 build bucket", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.EC2Fleet.S3Bucket = "example-custom-build-storage"
		items := purgeItems(cfg)
		joined := strings.Join(items, "\n")
		if !strings.Contains(joined, "example-custom-build-storage") {
			t.Errorf("purgeItems should confirm the custom bucket that will be purged; got %v", items)
		}
		if strings.Contains(joined, "ludus-builds-") {
			t.Errorf("purgeItems should not offer the default bucket when a custom one is configured; got %v", items)
		}
	})

	t.Run("falls back when unset", func(t *testing.T) {
		items := purgeItems(&config.Config{})
		joined := strings.Join(items, "\n")
		if !strings.Contains(joined, "ludus-server") {
			t.Errorf("purgeItems should fall back to default ECR repo; got %v", items)
		}
		if !strings.Contains(joined, "<account-id>") {
			t.Errorf("purgeItems should show an account-id placeholder when unset; got %v", items)
		}
	})
}

func TestResolveECRRepo(t *testing.T) {
	tests := []struct {
		name string
		repo string
		want string
	}{
		{"configured", "lyra-server-x86", "lyra-server-x86"},
		{"unset falls back to default", "", "ludus-server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.AWS.ECRRepository = tt.repo
			if got := resolveECRRepo(cfg); got != tt.want {
				t.Errorf("resolveECRRepo = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCleanupECRRepos(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		ecrErr   error
		wantRepo string
		wantErr  string
	}{
		{"configured repo, success", "lyra-server-x86", nil, "lyra-server-x86", ""},
		{"unset falls back to default", "", nil, "ludus-server", ""},
		{"deletion failure is printed and returned", "lyra-server-x86", errors.New("boom"), "lyra-server-x86", "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockCleaner{ecrErr: tt.ecrErr}
			cfg := &config.Config{}
			cfg.AWS.ECRRepository = tt.repo
			err := cleanupECRRepos(context.Background(), m, cfg)
			if m.ecrRepo != tt.wantRepo || m.ecrCalls != 1 {
				t.Errorf("cleanupECRRepos = repo %q calls %d, want %q / 1", m.ecrRepo, m.ecrCalls, tt.wantRepo)
			}
			verifyCleanupResult(t, err, tt.wantErr, tt.wantRepo)
		})
	}
}

// verifyCleanupResult checks a cleanup helper's error contract: nil on
// success, a contextual error naming the resource on failure.
func verifyCleanupResult(t *testing.T, err error, wantErr, resource string) {
	t.Helper()
	if wantErr == "" {
		if err != nil {
			t.Errorf("cleanup error = %v, want nil", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), wantErr) || !strings.Contains(err.Error(), resource) {
		t.Errorf("cleanup error = %v, want contextual failure naming %q", err, resource)
	}
}

func TestCleanupS3Bucket(t *testing.T) {
	tests := []struct {
		name     string
		s3Bucket string
		s3Err    error
		want     string
		wantErr  string
	}{
		{"success deletes the default bucket", "", nil, buildBucketName("123456789012"), ""},
		{"custom bucket is the purge target", "example-custom-build-storage", nil, "example-custom-build-storage", ""},
		{"deletion failure is printed and returned", "", errors.New("boom"), buildBucketName("123456789012"), "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockCleaner{s3Err: tt.s3Err}
			cfg := &config.Config{}
			cfg.AWS.AccountID = "123456789012"
			cfg.EC2Fleet.S3Bucket = tt.s3Bucket
			err := cleanupS3Bucket(context.Background(), m, aws.Config{}, cfg)
			if m.bucket != tt.want || m.s3Calls != 1 {
				t.Errorf("cleanupS3Bucket = bucket %q calls %d, want %q / 1", m.bucket, m.s3Calls, tt.want)
			}
			verifyCleanupResult(t, err, tt.wantErr, tt.want)
		})
	}
}

// TestCleanupS3BucketSkipsWhenUnresolvable pins the skip: when no custom
// bucket is configured and the account ID can't be resolved, the purge must
// not guess a bucket name.
func TestCleanupS3BucketSkipsWhenUnresolvable(t *testing.T) {
	m := &mockCleaner{}
	cfg := &config.Config{} // no account ID, no custom bucket

	if err := cleanupS3Bucket(context.Background(), m, aws.Config{}, cfg); err != nil {
		t.Fatalf("cleanupS3Bucket() error = %v, want nil skip", err)
	}
	if m.s3Calls != 0 {
		t.Errorf("DeleteS3Bucket calls = %d, want 0 when the bucket can't be resolved", m.s3Calls)
	}
}

// TestCleanupDurableArtifacts pins the --purge aggregation: every deletion
// failure is collected (and printed) and the aggregate names them all, so
// automation sees a non-zero exit instead of unconditional success.
func TestCleanupDurableArtifacts(t *testing.T) {
	tests := []struct {
		name    string
		ecrErr  error
		s3Err   error
		wantNil bool
		want    []string
	}{
		{"clean purge returns nil", nil, nil, true, nil},
		{"ECR failure surfaces", errors.New("ecr boom"), nil, false, []string{"ecr boom", "ECR"}},
		{"S3 failure surfaces", nil, errors.New("s3 boom"), false, []string{"s3 boom", "S3"}},
		{"both failures are listed", errors.New("ecr boom"), errors.New("s3 boom"), false, []string{"ecr boom", "s3 boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockCleaner{ecrErr: tt.ecrErr, s3Err: tt.s3Err}
			cfg := &config.Config{}
			cfg.AWS.AccountID = "123456789012"

			var err error
			out := captureStdout(func() {
				err = cleanupDurableArtifacts(context.Background(), m, aws.Config{}, cfg)
			})
			switch {
			case tt.wantNil && err != nil:
				t.Fatalf("cleanupDurableArtifacts() error = %v, want nil", err)
			case !tt.wantNil && err == nil:
				t.Fatal("cleanupDurableArtifacts() error = nil, want aggregated purge failure")
			case err != nil:
				verifyPurgeAggregate(t, err, out, tt.want)
			}
		})
	}
}

// verifyPurgeAggregate checks that the aggregated purge error lists every
// failure and that each was still printed for the user.
func verifyPurgeAggregate(t *testing.T, err error, out string, want []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("cleanupDurableArtifacts() error = %v, want it to mention %q", err, w)
		}
	}
	if !strings.Contains(out, "(continuing)") {
		t.Errorf("cleanupDurableArtifacts() output %q missing per-failure print", out)
	}
}
