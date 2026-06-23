package deploy

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

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
		cfg.AWS.AccountID = "575108928122"
		items := purgeItems(cfg)
		joined := strings.Join(items, "\n")
		if !strings.Contains(joined, "lyra-server-x86") {
			t.Errorf("purgeItems should name the ECR repo; got %v", items)
		}
		if !strings.Contains(joined, "ludus-builds-575108928122") {
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
