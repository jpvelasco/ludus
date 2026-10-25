package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

// TestHandleDeployFleetResolveError covers the ResolveTarget failure branch
// (tools_deploy.go:176-179).
func TestHandleDeployFleetResolveError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return nil, errors.New("resolve exploded")
	})

	result, _, err := handleDeployFleet(context.Background(), nil, deployFleetInput{})
	if err != nil {
		t.Fatalf("handleDeployFleet() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "could not resolve deploy target") {
		t.Errorf("result = %q, want resolution failure", text)
	}
}

// TestHandleDeployFleetDeployError covers the target.Deploy failure branch
// (tools_deploy.go:208-211).
func TestHandleDeployFleetDeployError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{
		AWS:       config.AWSConfig{ECRRepository: "ludus-server", AccountID: "123456789012", Region: "us-west-2"},
		Container: config.ContainerConfig{Tag: "5.7.3"},
	})

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "gamelift", err: errors.New("fleet deploy failed")}, nil
	})

	result, _, err := handleDeployFleet(context.Background(), nil, deployFleetInput{})
	if err != nil {
		t.Fatalf("handleDeployFleet() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "fleet deployment failed") {
		t.Errorf("result = %q, want fleet deployment failure", text)
	}
}

// TestHandleDeployStackAutoSwitchAndEnvError covers the pricing.AutoSwitch branch
// (tools_deploy.go:234-236) and the AWS env resolution failure (244-246). The
// mismatched arch/instance type triggers the switch before the env resolve fails.
func TestHandleDeployStackAutoSwitchAndEnvError(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AWS_PROFILE", "ludus-does-not-exist-anywhere")
	t.Setenv("AWS_SDK_LOAD_CONFIG", "1")

	cfg := &config.Config{
		Game:     config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject", Arch: "arm64"},
		GameLift: config.GameLiftConfig{FleetName: "testfleet", InstanceType: "c5.large"},
	}
	globals.SetGlobals(t, cfg)

	result, _, err := handleDeployStack(context.Background(), nil, deployStackInput{})
	if err != nil {
		t.Fatalf("handleDeployStack() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "could not resolve AWS environment") {
		t.Errorf("result = %q, want AWS environment failure", text)
	}
}

// TestHandleDeployAnywhereResolveError covers the anywhere ResolveTarget branch
// (tools_deploy.go:297-299).
func TestHandleDeployAnywhereResolveError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return nil, errors.New("resolve exploded")
	})

	result, _, err := handleDeployAnywhere(context.Background(), nil, deployAnywhereInput{})
	if err != nil {
		t.Fatalf("handleDeployAnywhere() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "could not resolve anywhere target") {
		t.Errorf("result = %q, want anywhere resolution failure", text)
	}
}

// TestHandleDeployAnywhereDeployError covers the anywhere Deploy failure branch
// (tools_deploy.go:317-320).
func TestHandleDeployAnywhereDeployError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "anywhere", err: errors.New("anywhere deploy failed")}, nil
	})

	result, _, err := handleDeployAnywhere(context.Background(), nil, deployAnywhereInput{})
	if err != nil {
		t.Fatalf("handleDeployAnywhere() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "anywhere deployment failed") {
		t.Errorf("result = %q, want anywhere deployment failure", text)
	}
}

// TestHandleDeployEC2ResolveError covers the ec2 ResolveTarget branch
// (tools_deploy.go:343-345).
func TestHandleDeployEC2ResolveError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return nil, errors.New("resolve exploded")
	})

	result, _, err := handleDeployEC2(context.Background(), nil, deployEC2Input{})
	if err != nil {
		t.Fatalf("handleDeployEC2() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "could not resolve ec2 target") {
		t.Errorf("result = %q, want ec2 resolution failure", text)
	}
}

// TestHandleDeployEC2DeployError covers the ec2 Deploy failure branch
// (tools_deploy.go:363-366).
func TestHandleDeployEC2DeployError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "ec2", err: errors.New("ec2 deploy failed")}, nil
	})

	result, _, err := handleDeployEC2(context.Background(), nil, deployEC2Input{})
	if err != nil {
		t.Fatalf("handleDeployEC2() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "EC2 fleet deployment failed") {
		t.Errorf("result = %q, want ec2 deployment failure", text)
	}
}

// TestHandleDeployEC2ReadsStateFleet covers the state EC2Fleet read after a
// successful deploy (tools_deploy.go:374-378).
func TestHandleDeployEC2ReadsStateFleet(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{
		Game: config.GameConfig{ProjectName: "Lyra", ProjectPath: "Lyra.uproject", Arch: "amd64"},
	})

	if err := state.UpdateEC2Fleet(&state.EC2FleetState{FleetID: "fleet-ec2-123", BuildID: "build-ec2-456"}); err != nil {
		t.Fatalf("state.UpdateEC2Fleet: %v", err)
	}

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "ec2", result: &deploy.DeployResult{TargetName: "ec2", Status: "CREATED"}}, nil
	})

	result, _, err := handleDeployEC2(context.Background(), nil, deployEC2Input{})
	if err != nil {
		t.Fatalf("handleDeployEC2() error = %v", err)
	}
	text := toolResultText(t, result)
	if !strings.Contains(text, "fleet-ec2-123") || !strings.Contains(text, "build-ec2-456") {
		t.Errorf("result = %q, want fleet and build IDs from state", text)
	}
}

// TestHandleDeploySessionResolveError covers the ResolveSessionTarget failure
// branch (tools_deploy.go:389-391).
func TestHandleDeploySessionResolveError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{Deploy: config.DeployConfig{Target: "gamelift"}})

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return nil, errors.New("resolve exploded")
	})

	result, _, err := handleDeploySession(context.Background(), nil, deploySessionInput{})
	if err != nil {
		t.Fatalf("handleDeploySession() error = %v", err)
	}
	if text := toolResultText(t, result); !strings.Contains(text, "could not resolve deploy target") {
		t.Errorf("result = %q, want session resolution failure", text)
	}
}

// TestRunDestroyForMCPDestroyError covers the single-target Destroy failure
// branch (tools_deploy.go:456-458).
func TestRunDestroyForMCPDestroyError(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{}

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "gamelift", err: errors.New("destroy exploded")}, nil
	})

	err := runDestroyForMCP(context.Background(), cfg, deployDestroyInput{Target: "gamelift"}, newToolOutput())
	if err == nil || !strings.Contains(err.Error(), "destroy exploded") {
		t.Fatalf("runDestroyForMCP() error = %v, want destroy failure", err)
	}
}

// TestDestroyAllTargetsContinuesOnDestroyError covers destroyAllTargets
// continuing past a Destroy error while surfacing it as an aggregate instead
// of returning unconditional success.
func TestDestroyAllTargetsContinuesOnDestroyError(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{}

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "target", err: errors.New("destroy exploded")}, nil
	})

	err := runDestroyForMCP(context.Background(), cfg, deployDestroyInput{AllTargets: true}, newToolOutput())
	if err == nil {
		t.Fatal("runDestroyForMCP(all) error = nil, want the sweep failure to surface")
	}
	for _, want := range []string{"destroy sweep failed for 5 target(s)", "destroy exploded"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("runDestroyForMCP(all) error = %v, want it to mention %q", err, want)
		}
	}
}

// TestRunDestroyForMCPAllTargetsPurgeAggregates pins the MCP full-wipe path:
// sweep and purge failures must both surface in one error.
func TestRunDestroyForMCPAllTargetsPurgeAggregates(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{}
	t.Setenv("AWS_PROFILE", "ludus-no-such-profile")

	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		return &testDeployTarget{name: "target", err: errors.New("destroy exploded")}, nil
	})

	err := runDestroyForMCP(context.Background(), cfg, deployDestroyInput{AllTargets: true, Purge: true}, newToolOutput())
	if err == nil {
		t.Fatal("runDestroyForMCP(all+purge) error = nil, want aggregated failures")
	}
	for _, want := range []string{"destroy sweep failed", "loading AWS config"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("runDestroyForMCP(all+purge) error = %v, want it to mention %q", err, want)
		}
	}
}

// TestMCPCleanupSharedResourcesLoadFailure pins the MCP purge wiring: a
// configuration failure must surface as an error, not be swallowed.
func TestMCPCleanupSharedResourcesLoadFailure(t *testing.T) {
	t.Setenv("AWS_PROFILE", "ludus-no-such-profile")

	err := cleanupSharedResources(context.Background(), &config.Config{}, newToolOutput())
	if err == nil || !strings.Contains(err.Error(), "loading AWS config") {
		t.Fatalf("cleanupSharedResources() error = %v, want load failure", err)
	}
}

// TestMCPCleanupECRReposReportsDeletionError pins the MCP ECR cleanup helper:
// a deletion failure must be printed and returned instead of swallowed.
func TestMCPCleanupECRReposReportsDeletionError(t *testing.T) {
	c := &mockDurableCleaner{ecrErr: errors.New("ecr boom")}
	cfg := &config.Config{}
	cfg.AWS.ECRRepository = "lyra-server-x86"

	err := cleanupECRRepos(context.Background(), c, cfg, newToolOutput())
	if err == nil || !strings.Contains(err.Error(), "ecr boom") || !strings.Contains(err.Error(), "lyra-server-x86") {
		t.Fatalf("cleanupECRRepos() error = %v, want contextual deletion failure", err)
	}
	if c.ecrRepo != "lyra-server-x86" {
		t.Errorf("DeleteECRRepository called with %q, want lyra-server-x86", c.ecrRepo)
	}
}

// TestMCPCleanupS3BucketReportsDeletionError pins the MCP S3 cleanup helper.
func TestMCPCleanupS3BucketReportsDeletionError(t *testing.T) {
	c := &mockDurableCleaner{s3Err: errors.New("s3 boom")}
	cfg := &config.Config{}
	cfg.AWS.AccountID = "123456789012"

	err := cleanupS3Bucket(context.Background(), c, aws.Config{}, cfg, newToolOutput())
	if err == nil || !strings.Contains(err.Error(), "s3 boom") {
		t.Fatalf("cleanupS3Bucket() error = %v, want deletion failure", err)
	}
	if c.bucket != "ludus-builds-123456789012" {
		t.Errorf("DeleteS3Bucket called with %q, want the account-derived bucket", c.bucket)
	}
}

// TestMCPCleanupS3BucketSkipsUnresolvedAccount pins the skip: when the
// account ID can't be resolved, no bucket is guessed.
func TestMCPCleanupS3BucketSkipsUnresolvedAccount(t *testing.T) {
	c := &mockDurableCleaner{}
	// A zero-value AWS config fails STS resolution immediately (no region),
	// so the skip path is driven hermetically.

	if err := cleanupS3Bucket(context.Background(), c, aws.Config{}, &config.Config{}, newToolOutput()); err != nil {
		t.Fatalf("cleanupS3Bucket() error = %v, want nil skip", err)
	}
	if c.bucket != "" {
		t.Errorf("DeleteS3Bucket called with %q, want no call", c.bucket)
	}
}

// TestMCPCleanupS3BucketUsesConfiguredBucket pins the #714 analog in the MCP
// sweep: a configured ec2fleet.s3Bucket is deleted instead of the
// account-derived default.
func TestMCPCleanupS3BucketUsesConfiguredBucket(t *testing.T) {
	c := &mockDurableCleaner{}
	cfg := &config.Config{AWS: config.AWSConfig{AccountID: "123456789012"}}
	cfg.EC2Fleet.S3Bucket = "example-custom-build-storage"

	if err := cleanupS3Bucket(context.Background(), c, aws.Config{}, cfg, newToolOutput()); err != nil {
		t.Fatalf("cleanupS3Bucket() error = %v, want nil", err)
	}
	if c.bucket != "example-custom-build-storage" {
		t.Errorf("DeleteS3Bucket called with %q, want the configured bucket", c.bucket)
	}
}

// TestMCPCleanupECRReposFallsBackToDefault pins the default repository name
// when none is configured.
func TestMCPCleanupECRReposFallsBackToDefault(t *testing.T) {
	c := &mockDurableCleaner{}

	if err := cleanupECRRepos(context.Background(), c, &config.Config{}, newToolOutput()); err != nil {
		t.Fatalf("cleanupECRRepos() error = %v, want nil", err)
	}
	if c.ecrRepo != "ludus-server" {
		t.Errorf("DeleteECRRepository called with %q, want ludus-server", c.ecrRepo)
	}
}

// mockDurableCleaner records the durable-artifact deletions the MCP purge
// path requests, without touching AWS.
type mockDurableCleaner struct {
	ecrRepo string
	bucket  string
	ecrErr  error
	s3Err   error
}

func (m *mockDurableCleaner) DeleteECRRepository(_ context.Context, repoName string) error {
	m.ecrRepo = repoName
	return m.ecrErr
}

func (m *mockDurableCleaner) DeleteS3Bucket(_ context.Context, bucketName string) error {
	m.bucket = bucketName
	return m.s3Err
}
