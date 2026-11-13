package cleanup

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

func TestPurgeScopeUsesOnlyConfiguredDurableNames(t *testing.T) {
	for _, tt := range []struct{ name, repo, bucket, account, wantRepo, wantBucket, decision string }{
		{"defaults", "", "", "123456789012", "ludus-server", "ludus-builds-123456789012", "planned"},
		{"unresolved account", "", "", "", "ludus-server", "", "unresolved"},
		{"custom without account", "server", "custom-bucket", "", "server", "custom-bucket", "planned"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.AWS.ECRRepository, cfg.AWS.AccountID = tt.repo, tt.account
			cfg.EC2Fleet.S3Bucket, cfg.Engine.DockerImageName = tt.bucket, "engine-do-not-purge"
			items := PlanPurge(cfg)
			if len(items) != 2 {
				t.Fatalf("purge scope has %d items", len(items))
			}
			if items[0].Identifier != tt.wantRepo || items[1].Identifier != tt.wantBucket || items[1].Decision != tt.decision {
				t.Fatalf("purge scope = %+v", items)
			}
		})
	}
}
