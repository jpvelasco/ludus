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
	}{
		{"configured repo, success", "lyra-server-x86", nil, "lyra-server-x86"},
		{"unset falls back to default", "", nil, "ludus-server"},
		{"error path prints and continues", "lyra-server-x86", errors.New("boom"), "lyra-server-x86"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockCleaner{ecrErr: tt.ecrErr}
			cfg := &config.Config{}
			cfg.AWS.ECRRepository = tt.repo
			cleanupECRRepos(context.Background(), m, cfg)
			if m.ecrRepo != tt.wantRepo || m.ecrCalls != 1 {
				t.Errorf("cleanupECRRepos = repo %q calls %d, want %q / 1", m.ecrRepo, m.ecrCalls, tt.wantRepo)
			}
		})
	}
}

func TestCleanupS3Bucket(t *testing.T) {
	tests := []struct {
		name  string
		s3Err error
		want  string
	}{
		{"success", nil, buildBucketName("123456789012")},
		{"error path prints and continues", errors.New("boom"), buildBucketName("123456789012")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockCleaner{s3Err: tt.s3Err}
			cfg := &config.Config{}
			cfg.AWS.AccountID = "123456789012"
			cleanupS3Bucket(context.Background(), m, aws.Config{}, cfg)
			if m.bucket != tt.want || m.s3Calls != 1 {
				t.Errorf("cleanupS3Bucket = bucket %q calls %d, want %q / 1", m.bucket, m.s3Calls, tt.want)
			}
		})
	}
}
