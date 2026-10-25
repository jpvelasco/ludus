package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// extendedStubTarget is the existing stubTarget with error-injection fields.
// This replaces the previous stubTarget for deploy test error scenarios.
type extendedStubTarget struct {
	name             string
	caps             deploy.Capabilities
	deployErr        error
	statusErr        error
	destroyErr       error
	deployResult     *deploy.DeployResult
	statusResult     *deploy.DeployStatus
	supportsSession  bool
	createSessionErr error
	sessionResult    *deploy.SessionInfo
	lastInput        deploy.DeployInput
}

func (t *extendedStubTarget) Name() string                      { return t.name }
func (t *extendedStubTarget) Capabilities() deploy.Capabilities { return t.caps }

func (t *extendedStubTarget) Deploy(_ context.Context, input deploy.DeployInput) (*deploy.DeployResult, error) {
	t.lastInput = input
	if t.deployErr != nil {
		return nil, t.deployErr
	}
	if t.deployResult != nil {
		return t.deployResult, nil
	}
	return &deploy.DeployResult{
		TargetName: t.name,
		Status:     "active",
		Detail:     "deployed",
	}, nil
}

func (t *extendedStubTarget) Status(_ context.Context) (*deploy.DeployStatus, error) {
	if t.statusErr != nil {
		return nil, t.statusErr
	}
	if t.statusResult != nil {
		return t.statusResult, nil
	}
	return &deploy.DeployStatus{
		TargetName: t.name,
		Status:     "active",
	}, nil
}

func (t *extendedStubTarget) Destroy(_ context.Context) error {
	return t.destroyErr
}

func (t *extendedStubTarget) CreateSession(_ context.Context, _ int) (*deploy.SessionInfo, error) {
	if t.createSessionErr != nil {
		return nil, t.createSessionErr
	}
	if t.sessionResult != nil {
		return t.sessionResult, nil
	}
	return &deploy.SessionInfo{
		SessionID: "sess-123",
		IPAddress: "10.0.0.1",
		Port:      7777,
	}, nil
}

func (t *extendedStubTarget) DescribeSession(_ context.Context, sessionID string) (string, error) {
	return "active", nil
}

func TestStageDeployDryRun(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "my-game",
		},
		Container: config.ContainerConfig{
			ImageName:  "test-image",
			Tag:        "v1.0",
			ServerPort: 7777,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
		GameLift: config.GameLiftConfig{
			InstanceType: "c5.large",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:            cfg,
		r:              r,
		arch:           "amd64",
		serverBuildDir: "C:/build",
		target: &extendedStubTarget{
			name: "gamelift",
			caps: deploy.Capabilities{NeedsImageURI: true},
		},
	}

	err := p.stageDeploy(context.Background())
	if err != nil {
		t.Fatalf("stageDeploy() error = %v, want nil", err)
	}
}

func TestStageDeploySuccess(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "my-game",
		},
		Container: config.ContainerConfig{
			ImageName:  "test-image",
			Tag:        "v1.0",
			ServerPort: 7777,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
		GameLift: config.GameLiftConfig{
			InstanceType: "c5.large",
		},
	}

	globals.SetGlobals(t, cfg)

	target := &extendedStubTarget{
		name: "gamelift",
		caps: deploy.Capabilities{NeedsImageURI: true, SupportsDeploy: true},
		deployResult: &deploy.DeployResult{
			TargetName: "gamelift",
			Status:     "active",
			Detail:     "fleet-123",
		},
	}

	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:            cfg,
		r:              r,
		arch:           "amd64",
		serverBuildDir: "C:/build",
		target:         target,
	}

	err := p.stageDeploy(context.Background())
	if err != nil {
		t.Fatalf("stageDeploy() error = %v, want nil", err)
	}
}

func TestStageDeployError(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "my-game",
		},
		Container: config.ContainerConfig{
			ImageName: "test-image",
			Tag:       "v1.0",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg)

	target := &extendedStubTarget{
		name:      "gamelift",
		deployErr: fmt.Errorf("deployment failed"),
	}

	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:            cfg,
		r:              r,
		arch:           "amd64",
		serverBuildDir: "C:/build",
		target:         target,
	}

	err := p.stageDeploy(context.Background())
	if err == nil {
		t.Fatal("stageDeploy() expected error, got nil")
	}
}

func TestStageSessionDryRun(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	target := &extendedStubTarget{
		name:            "gamelift",
		caps:            deploy.Capabilities{SupportsSession: true},
		supportsSession: true,
	}

	p := &pipelineCtx{
		cfg:    cfg,
		r:      r,
		target: target,
	}

	err := p.stageSession(context.Background())
	if err != nil {
		t.Fatalf("stageSession() error = %v, want nil", err)
	}
}

func TestStageSessionSuccess(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg)

	r, _ := testsupport.RecordingRunner()

	target := &extendedStubTarget{
		name:            "gamelift",
		caps:            deploy.Capabilities{SupportsSession: true},
		supportsSession: true,
		sessionResult: &deploy.SessionInfo{
			SessionID: "session-456",
			IPAddress: "192.168.1.100",
			Port:      7778,
		},
	}

	p := &pipelineCtx{
		cfg:    cfg,
		r:      r,
		target: target,
	}

	err := p.stageSession(context.Background())
	if err != nil {
		t.Fatalf("stageSession() error = %v, want nil", err)
	}
}

func TestStageSessionTargetNotSupported(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg)

	r, _ := testsupport.RecordingRunner()

	// Use stubTarget which does NOT implement SessionManager interface
	target := &stubTarget{
		name: "binary",
	}

	p := &pipelineCtx{
		cfg:    cfg,
		r:      r,
		target: target,
	}

	err := p.stageSession(context.Background())
	if err == nil {
		t.Fatal("stageSession() expected error for unsupported target, got nil")
	}
}

func TestStageSessionError(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg)

	r, _ := testsupport.RecordingRunner()

	target := &extendedStubTarget{
		name:             "gamelift",
		supportsSession:  true,
		createSessionErr: fmt.Errorf("session creation failed"),
	}

	p := &pipelineCtx{
		cfg:    cfg,
		r:      r,
		target: target,
	}

	err := p.stageSession(context.Background())
	if err == nil {
		t.Fatal("stageSession() expected error, got nil")
	}
}

func TestBuildImageURIDryRun(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "my-game",
		},
		Container: config.ContainerConfig{
			Tag: "v1.0.0",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	p := &pipelineCtx{
		cfg: cfg,
	}

	uri, err := p.buildImageURI(context.Background())
	if err != nil {
		t.Fatalf("buildImageURI() error = %v, want nil", err)
	}

	if uri == "" {
		t.Errorf("buildImageURI() returned empty URI")
	}
}

// TestStageDeployDryRunFileTarget previews a file-based deploy without an
// image reference: the target consumes server files only, so the dry-run
// output must name the target instead of an image.
func TestStageDeployDryRunFileTarget(t *testing.T) {
	cfg := &config.Config{
		Container: config.ContainerConfig{
			ServerPort: 7777,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:            cfg,
		r:              r,
		arch:           "amd64",
		serverBuildDir: filepath.Join(t.TempDir(), "out"),
		target:         &extendedStubTarget{name: "binary"},
	}

	out := captureStdout(func() {
		if err := p.stageDeploy(context.Background()); err != nil {
			t.Errorf("stageDeploy() error = %v, want nil", err)
		}
	})

	if !strings.Contains(out, "(dry run) would deploy to binary") {
		t.Errorf("dry-run output must name the file target, got:\n%s", out)
	}
	if strings.Contains(out, "would deploy image") {
		t.Errorf("dry-run output must not claim an image deploy for a file target, got:\n%s", out)
	}
}

// TestStageDeployFileTargetNoAWS is the issue-693 regression: a file-based
// target (e.g. binary export) consumes only the packaged server directory, so
// the deploy stage must succeed with no AWS account/region and no ECR
// repository or image tag configured, and must hand the target an empty
// ImageURI.
func TestStageDeployFileTargetNoAWS(t *testing.T) {
	cfg := &config.Config{
		// AWS and Container.Tag intentionally zero: nothing here may be
		// resolved for a target that deploys server files only.
		Container: config.ContainerConfig{
			ImageName:  "test-image",
			ServerPort: 7777,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(false))

	target := &extendedStubTarget{name: "binary"}
	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:            cfg,
		r:              r,
		arch:           "amd64",
		serverBuildDir: filepath.Join(t.TempDir(), "out"),
		target:         target,
	}

	err := p.stageDeploy(context.Background())
	if err != nil {
		t.Fatalf("stageDeploy() error = %v, want nil (file targets need no AWS/ECR configuration)", err)
	}

	if target.lastInput.ImageURI != "" {
		t.Errorf("Deploy() ImageURI = %q, want empty for a file-based target", target.lastInput.ImageURI)
	}
	if target.lastInput.ServerBuildDir != p.serverBuildDir {
		t.Errorf("Deploy() ServerBuildDir = %q, want %q", target.lastInput.ServerBuildDir, p.serverBuildDir)
	}
}

// TestStageDeployImageTargetResolvesURI guards the other side of issue 693:
// a target that deploys the pushed container image still gets a resolved ECR
// image URI.
func TestStageDeployImageTargetResolvesURI(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "my-game",
		},
		Container: config.ContainerConfig{
			ImageName:  "test-image",
			Tag:        "v1.0",
			ServerPort: 7777,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(false))

	target := &extendedStubTarget{
		name: "gamelift",
		caps: deploy.Capabilities{NeedsImageURI: true},
	}
	r, _ := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:            cfg,
		r:              r,
		arch:           "amd64",
		serverBuildDir: filepath.Join(t.TempDir(), "out"),
		target:         target,
	}

	err := p.stageDeploy(context.Background())
	if err != nil {
		t.Fatalf("stageDeploy() error = %v, want nil", err)
	}

	const wantURI = "123456789012.dkr.ecr.us-east-1.amazonaws.com/my-game:v1.0"
	if target.lastInput.ImageURI != wantURI {
		t.Errorf("Deploy() ImageURI = %q, want %q", target.lastInput.ImageURI, wantURI)
	}
}
