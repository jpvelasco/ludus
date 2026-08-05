package deploy

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// fakeTarget is a configurable deploy.Target that also implements
// deploy.SessionManager, so deploy commands can be exercised without AWS.
type fakeTarget struct {
	name         string
	deployErr    error
	destroyErr   error
	sessionErr   error
	deployResult *deploy.DeployResult
	sessionInfo  *deploy.SessionInfo
	sessionCalls int
	destroyCalls int
}

func (f *fakeTarget) Name() string                      { return f.name }
func (f *fakeTarget) Capabilities() deploy.Capabilities { return deploy.Capabilities{} }

func (f *fakeTarget) Deploy(_ context.Context, _ deploy.DeployInput) (*deploy.DeployResult, error) {
	if f.deployErr != nil {
		return nil, f.deployErr
	}
	if f.deployResult != nil {
		return f.deployResult, nil
	}
	return &deploy.DeployResult{TargetName: f.name, Status: "active", Detail: f.name + "-detail"}, nil
}

func (f *fakeTarget) Status(_ context.Context) (*deploy.DeployStatus, error) {
	return &deploy.DeployStatus{TargetName: f.name, Status: "active"}, nil
}

func (f *fakeTarget) Destroy(_ context.Context) error {
	f.destroyCalls++
	return f.destroyErr
}

func (f *fakeTarget) CreateSession(_ context.Context, _ int) (*deploy.SessionInfo, error) {
	f.sessionCalls++
	if f.sessionErr != nil {
		return nil, f.sessionErr
	}
	if f.sessionInfo != nil {
		return f.sessionInfo, nil
	}
	return &deploy.SessionInfo{SessionID: "sess-1", IPAddress: "10.0.0.1", Port: 7777}, nil
}

func (f *fakeTarget) DescribeSession(_ context.Context, _ string) (string, error) {
	return "active", nil
}

// nonSessionTarget implements deploy.Target but not deploy.SessionManager.
type nonSessionTarget struct {
	name string
}

func (n *nonSessionTarget) Name() string                      { return n.name }
func (n *nonSessionTarget) Capabilities() deploy.Capabilities { return deploy.Capabilities{} }

func (n *nonSessionTarget) Deploy(_ context.Context, _ deploy.DeployInput) (*deploy.DeployResult, error) {
	return &deploy.DeployResult{TargetName: n.name, Status: "active", Detail: n.name + "-detail"}, nil
}

func (n *nonSessionTarget) Status(_ context.Context) (*deploy.DeployStatus, error) {
	return &deploy.DeployStatus{TargetName: n.name, Status: "active"}, nil
}

func (n *nonSessionTarget) Destroy(_ context.Context) error { return nil }

// swapTargetFactory installs a fake deploy.Target factory and restores the
// original on test cleanup.
func swapTargetFactory(t *testing.T, fn func(context.Context, *config.Config, string) (deploy.Target, error)) {
	t.Helper()
	globals.SwapResolveTarget(t, fn)
}

// newCommand returns a cobra command with a background context.
func newCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

// captureStdout captures os.Stdout while fn runs and returns what was written.
func captureStdout(fn func()) string {
	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()

	r, w, err := os.Pipe()
	if err != nil {
		return ""
	}
	defer func() { _ = r.Close() }()

	os.Stdout = w
	var buf bytes.Buffer
	done := make(chan bool, 1)
	go func() {
		_, _ = io.Copy(&buf, r)
		done <- true
	}()

	fn()

	_ = w.Close()
	<-done
	return buf.String()
}

// deployFlags mirrors the package flag vars so tests can restore them.
type deployFlags struct {
	region, instanceType, fleetName, targetFlag           string
	stackName, anywhereIP, ec2Arch                        string
	withSession, destroyAllTgts, destroyPurge, destroyYes bool
}

// stubAWSCLI puts a fake `aws` executable on PATH so the prereq AWS-readiness
// check (checkAWSCredentials shells out to `aws sts get-caller-identity`)
// resolves to the stub instead of the real CLI, keeping deploy tests hermetic.
func stubAWSCLI(t *testing.T) {
	t.Helper()
	testsupport.FakeTool(t, "aws", testsupport.ToolBehavior{ExitCode: 1})
}

// saveDeployFlags snapshots the package flag vars and restores them on cleanup.
func saveDeployFlags(t *testing.T) {
	t.Helper()
	stubAWSCLI(t)
	orig := deployFlags{
		region, instanceType, fleetName, targetFlag,
		stackName, anywhereIP, ec2Arch,
		withSession, destroyAllTgts, destroyPurge, destroyYes,
	}
	t.Cleanup(func() {
		region, instanceType, fleetName, targetFlag = orig.region, orig.instanceType, orig.fleetName, orig.targetFlag
		stackName, anywhereIP, ec2Arch = orig.stackName, orig.anywhereIP, orig.ec2Arch
		withSession, destroyAllTgts, destroyPurge, destroyYes = orig.withSession, orig.destroyAllTgts, orig.destroyPurge, orig.destroyYes
	})
}

// blockStateWrites makes state.Load/Save fail by placing a file where the
// .ludus state directory should be.
func blockStateWrites(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, ".ludus"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("create .ludus blocker: %v", err)
	}
}
