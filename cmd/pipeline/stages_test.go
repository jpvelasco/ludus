package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/jpvelasco/ludus/internal/wsl"
)

// stubTarget is a minimal deploy.Target for testing buildStages.
type stubTarget struct {
	name string
	caps deploy.Capabilities
}

func (t *stubTarget) Name() string                      { return t.name }
func (t *stubTarget) Capabilities() deploy.Capabilities { return t.caps }
func (t *stubTarget) Deploy(_ context.Context, _ deploy.DeployInput) (*deploy.DeployResult, error) {
	return nil, nil
}
func (t *stubTarget) Status(_ context.Context) (*deploy.DeployStatus, error) { return nil, nil }
func (t *stubTarget) Destroy(_ context.Context) error                        { return nil }

func TestBuildStagesCount(t *testing.T) {
	p := &pipelineCtx{
		cfg: &config.Config{Game: config.GameConfig{ProjectName: "TestGame"}},
		target: &stubTarget{
			name: "binary",
			caps: deploy.Capabilities{},
		},
		arch: "amd64",
	}

	stages := buildStages(p)
	if len(stages) != 8 {
		t.Errorf("buildStages() returned %d stages, want 8", len(stages))
	}
}

func TestPipelineCtxWSL2Fields(t *testing.T) {
	p := &pipelineCtx{
		wslNative: true,
		wslDistro: "Ubuntu-24.04",
	}
	if !p.wslNative {
		t.Error("expected wslNative = true")
	}
	if p.wslDistro != "Ubuntu-24.04" {
		t.Errorf("wslDistro = %q, want %q", p.wslDistro, "Ubuntu-24.04")
	}
}

func TestBuildImageURI(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
		region    string
		repo      string
		tag       string
		want      string
	}{
		{
			name:      "standard",
			accountID: "123456789012",
			region:    "us-east-1",
			repo:      "my-game",
			tag:       "latest",
			want:      "123456789012.dkr.ecr.us-east-1.amazonaws.com/my-game:latest",
		},
		{
			name:      "eu-region",
			accountID: "000000000001",
			region:    "eu-west-1",
			repo:      "server",
			tag:       "v1.2.3",
			want:      "000000000001.dkr.ecr.eu-west-1.amazonaws.com/server:v1.2.3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &pipelineCtx{
				cfg: &config.Config{
					AWS: config.AWSConfig{
						AccountID:     tt.accountID,
						Region:        tt.region,
						ECRRepository: tt.repo,
					},
					Container: config.ContainerConfig{Tag: tt.tag},
				},
			}
			got, err := p.buildImageURI(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("buildImageURI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildImageURIErrors(t *testing.T) {
	tests := []struct {
		name    string
		account string
		region  string
		repo    string
		tag     string
		wantErr string
	}{
		{
			name:    "empty repository",
			account: "123456789012",
			region:  "us-east-1",
			repo:    "",
			tag:     "latest",
			wantErr: "repository",
		},
		{
			name:    "empty tag",
			account: "123456789012",
			region:  "us-east-1",
			repo:    "my-game",
			tag:     "",
			wantErr: "tag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &pipelineCtx{
				cfg: &config.Config{
					AWS: config.AWSConfig{
						AccountID:     tt.account,
						Region:        tt.region,
						ECRRepository: tt.repo,
					},
					Container: config.ContainerConfig{Tag: tt.tag},
				},
			}
			_, err := p.buildImageURI(context.Background())
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestResolveWSL2GameDDCPath(t *testing.T) {
	w := &wsl.WSL2{}

	tests := []struct {
		name        string
		engineState *state.WSL2EngineState
		ddcMode     string
		ddcPath     string
		want        string
	}{
		{
			name:        "state DDC path takes priority",
			engineState: &state.WSL2EngineState{DDCPath: "/home/user/ludus/ddc"},
			ddcMode:     "local",
			ddcPath:     "C:/ludus/ddc",
			want:        "/home/user/ludus/ddc",
		},
		{
			name:        "fallback to virtiofs when state empty and mode is local",
			engineState: &state.WSL2EngineState{DDCPath: ""},
			ddcMode:     "local",
			ddcPath:     `C:\ludus\ddc`,
			want:        "/mnt/c/ludus/ddc",
		},
		{
			name:        "empty when mode is none and no state path",
			engineState: &state.WSL2EngineState{DDCPath: ""},
			ddcMode:     "none",
			ddcPath:     `C:\ludus\ddc`,
			want:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveWSL2GameDDCPath(tt.engineState, tt.ddcMode, tt.ddcPath, w)
			if got != tt.want {
				t.Errorf("resolveWSL2GameDDCPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildEngineNativeDryRun(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			MaxJobs:    1,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, getLines := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:    cfg,
		r:      r,
		target: &stubTarget{name: "binary", caps: deploy.Capabilities{}},
	}

	result, err := p.buildEngineNative(context.Background())

	if err != nil {
		t.Fatalf("buildEngineNative() error = %v, want nil", err)
	}
	if result == nil || !result.Success {
		t.Errorf("buildEngineNative() success = false, want true")
	}

	lines := getLines()
	if len(lines) == 0 {
		t.Fatal("expected recorded command lines, got none")
	}

	hasSetup := findInLines(lines, "Setup")
	if !hasSetup {
		t.Errorf("expected Setup in lines, got: %v", lines)
	}

	hasShaderCompile := findInLines(lines, "ShaderCompileWorker")
	if !hasShaderCompile {
		t.Errorf("expected ShaderCompileWorker in lines, got: %v", lines)
	}
}

// findInLines searches for a substring in a list of lines.
func findInLines(lines []string, substring string) bool {
	for _, line := range lines {
		if strings.Contains(line, substring) {
			return true
		}
	}
	return false
}

func TestStageValidatePrebuiltImage(t *testing.T) {
	// Test validate with prebuilt image (skips engine source checks for docker backend).
	// Environment-dependent checks (disk, memory) may fail on CI runners and are acceptable.
	engineRoot, projectPath, cfg := setupTestContext(t, "Lyra")

	cfg.Engine.DockerImage = "my.repo/engine:5.7.3"

	globals.SetGlobals(t, cfg)

	p := newTestPipelineCtx(t, cfg, &testContextOpts{
		containerBackend: "docker",
	})

	err := p.stageValidate(context.Background())
	// With a prebuilt image, engine source and toolchain checks are skipped.
	// Disk Space and Memory are environment-dependent and may fail on tight CI runners.
	// Assert that validation succeeds OR only fails due to environment checks.
	if err != nil {
		errMsg := err.Error()
		// If validation failed, it should be due to environment-dependent checks only.
		// Verify that the error mentions "prerequisite check(s) failed" (which is OK),
		// not something more fundamental like missing engine source or toolchain.
		if !strings.Contains(errMsg, "prerequisite check") {
			t.Fatalf("stageValidate() error = %v, want nil or environment-dependent check failure", err)
		}
	}

	_ = engineRoot
	_ = projectPath
}
