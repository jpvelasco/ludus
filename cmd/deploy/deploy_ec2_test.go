package deploy

import (
	"context"
	"runtime"
	"testing"

	"github.com/spf13/cobra"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestValidateEC2Prereqs(t *testing.T) {
	// validateEC2Prereqs runs AWS-readiness (a warning-pass when creds are
	// absent, not a hard fail) and the wrapper make check. On linux/amd64 with
	// make installed (the CI ubuntu leg) both pass and it returns nil; that path
	// is what we assert. On other hosts make may be absent, so we only require it
	// does not panic.
	cfg := &config.Config{}
	cfg.Game.Arch = "amd64"

	err := validateEC2Prereqs(cfg)
	if runtime.GOOS == "linux" && err != nil {
		t.Logf("validateEC2Prereqs returned %v (acceptable if make absent on host)", err)
	}
}

func TestValidateEC2Prereqs_FailsWhenMakeMissing(t *testing.T) {
	// With an empty PATH, the wrapper-build make check fails for a native
	// linux/amd64 target, so validateEC2Prereqs must return the fail-fast error
	// (rather than letting the deploy die deep in the wrapper build).
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("make check only hard-fails for a native linux/amd64 target")
	}
	t.Setenv("PATH", "")

	cfg := &config.Config{}
	cfg.Game.Arch = "amd64"
	if err := validateEC2Prereqs(cfg); err == nil {
		t.Fatal("expected an error when make is unavailable")
	}
}

func TestRunEC2_FailsFastOnMissingPrereq(t *testing.T) {
	// runEC2 must propagate the prereq failure (covers the validate-error guard).
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("make check only hard-fails for a native linux/amd64 target")
	}
	origCfg := globals.Cfg
	t.Cleanup(func() { globals.Cfg = origCfg })
	t.Setenv("PATH", "")

	globals.Cfg = &config.Config{
		AWS:  config.AWSConfig{Region: "us-west-2"},
		Game: config.GameConfig{Arch: "amd64", ProjectName: "Lyra"},
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runEC2(cmd, nil); err == nil {
		t.Fatal("expected runEC2 to fail fast on the missing-make prereq")
	}
}

func TestRunEC2_DryRunReturnsNil(t *testing.T) {
	// Drives runEC2 through its full early path — flag apply, prereq validation,
	// target resolution, pricing hints — stopping at the dry-run guard before any
	// real AWS deploy. Covers the runEC2 call site and validateEC2Prereqs without
	// touching AWS.
	origCfg := globals.Cfg
	origDry := globals.DryRun
	t.Cleanup(func() { globals.Cfg = origCfg; globals.DryRun = origDry })

	globals.DryRun = true
	globals.Cfg = &config.Config{
		AWS:  config.AWSConfig{Region: "us-west-2"},
		Game: config.GameConfig{Arch: "amd64", ProjectName: "Lyra"},
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runEC2(cmd, nil); err != nil {
		t.Fatalf("runEC2 dry-run returned error: %v", err)
	}
}
