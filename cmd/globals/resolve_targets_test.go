package globals

import (
	"context"
	"strings"
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

// TestResolveTarget_ImageURIEmptyTagError covers the ImageURI failure branch in
// resolveGameLift and resolveStack (resolve.go:89-91, 118-121): with a region
// and account ID configured, awsenv resolves offline and the empty container
// tag trips the URI builder.
func TestResolveTarget_ImageURIEmptyTagError(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{"gamelift", "gamelift"},
		{"stack", "stack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Deploy: config.DeployConfig{Target: tt.target},
				AWS:    config.AWSConfig{Region: "us-west-2", AccountID: "123456789012", ECRRepository: "ludus-server"},
			}
			SetGlobals(t, cfg, WithDryRun(true))

			target, err := ResolveTarget(context.Background(), cfg, "")
			if err == nil || !strings.Contains(err.Error(), "image tag is empty") {
				t.Fatalf("ResolveTarget() error = %v, want 'image tag is empty'; target = %v", err, target)
			}
		})
	}
}
