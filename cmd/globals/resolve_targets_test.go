package globals

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

// TestResolveTarget_AWSTargets exercises the target factory switch for every
// target that resolves AWS state. Dry-run keeps the awsenv resolver offline:
// the region comes from config, the account from the placeholder, and the
// deployer constructors are pure, so no network or credentials are touched.
func TestResolveTarget_AWSTargets(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		instanceType string
		arch         string
		projectPath  string
		wantName     string
		wantErr      bool
	}{
		{name: "empty defaults to gamelift", wantName: "gamelift"},
		{name: "gamelift", target: "gamelift", wantName: "gamelift"},
		{name: "gamelift arch mismatch", target: "gamelift", instanceType: "c7g.large", arch: "amd64", wantName: "gamelift"},
		{name: "stack", target: "stack", wantName: "stack"},
		{name: "stack arch mismatch", target: "stack", instanceType: "c7g.large", arch: "amd64", wantName: "stack"},
		{name: "anywhere", target: "anywhere", projectPath: "C:\\projects\\MyGame\\MyGame.uproject", wantName: "anywhere"},
		{name: "anywhere missing project path", target: "anywhere", wantErr: true},
		{name: "ec2", target: "ec2", wantName: "ec2"},
		{name: "ec2 arch mismatch", target: "ec2", instanceType: "c7g.large", arch: "amd64", wantName: "ec2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Deploy:    config.DeployConfig{Target: tt.target},
				Game:      config.GameConfig{Arch: tt.arch, ProjectPath: tt.projectPath},
				GameLift:  config.GameLiftConfig{InstanceType: tt.instanceType},
				AWS:       config.AWSConfig{Region: "us-west-2", AccountID: "123456789012", ECRRepository: "ludus-server"},
				Container: config.ContainerConfig{Tag: "latest"},
			}
			SetGlobals(t, cfg, WithDryRun(true))

			target, err := ResolveTarget(context.Background(), cfg, "")
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveTarget() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if target == nil {
				t.Fatal("ResolveTarget() returned nil target")
			}
			if got := target.Name(); got != tt.wantName {
				t.Errorf("target.Name() = %q, want %q", got, tt.wantName)
			}
		})
	}
}
