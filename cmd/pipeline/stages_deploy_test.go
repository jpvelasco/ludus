package pipeline

import (
	"context"
	"fmt"
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
}

func (t *extendedStubTarget) Name() string                      { return t.name }
func (t *extendedStubTarget) Capabilities() deploy.Capabilities { return t.caps }

func (t *extendedStubTarget) Deploy(_ context.Context, _ deploy.DeployInput) (*deploy.DeployResult, error) {
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
		target:         &extendedStubTarget{name: "gamelift"},
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
		caps: deploy.Capabilities{SupportsDeploy: true},
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
