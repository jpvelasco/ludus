package deploy

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
)

func testEC2Cfg() *config.Config {
	return &config.Config{
		AWS:       config.AWSConfig{Region: "us-west-2"},
		Game:      config.GameConfig{Arch: "amd64", ProjectName: "Lyra", ProjectPath: `C:\proj\Lyra.uproject`},
		GameLift:  config.GameLiftConfig{InstanceType: "c6i.large"},
		Container: config.ContainerConfig{ServerPort: 7777},
	}
}

func testAnywhereCfg() *config.Config {
	return &config.Config{
		AWS:       config.AWSConfig{Region: "us-east-1"},
		Game:      config.GameConfig{Arch: "amd64", ProjectName: "Lyra", ProjectPath: `C:\proj\Lyra.uproject`},
		GameLift:  config.GameLiftConfig{FleetName: "anywhere-fleet"},
		Anywhere:  config.AnywhereConfig{LocationName: "custom-test"},
		Container: config.ContainerConfig{ServerPort: 7777},
	}
}

func TestRunEC2MissingServerBuildDir(t *testing.T) {
	cfg := testEC2Cfg()
	cfg.Game.ProjectPath = ""
	globals.SetGlobals(t, cfg, globals.WithDryRun(false))
	saveDeployFlags(t)
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "ec2"}, nil
	})

	err := runEC2(newCommand(), nil)
	if err == nil {
		t.Fatal("runEC2() expected error for missing server build dir")
	}
	if !strings.Contains(err.Error(), "server build directory") {
		t.Errorf("runEC2() error = %v, want server build directory hint", err)
	}
}

func TestRunEC2ResolveError(t *testing.T) {
	globals.SetGlobals(t, testEC2Cfg(), globals.WithDryRun(false))
	saveDeployFlags(t)
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return nil, fmt.Errorf("resolve boom")
	})
	if err := runEC2(newCommand(), nil); err == nil {
		t.Fatal("runEC2() expected resolve error")
	}
}

func TestRunEC2DeployError(t *testing.T) {
	globals.SetGlobals(t, testEC2Cfg(), globals.WithDryRun(false))
	saveDeployFlags(t)
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "ec2", deployErr: fmt.Errorf("deploy boom")}, nil
	})

	err := runEC2(newCommand(), nil)
	if err == nil {
		t.Fatal("runEC2() expected deploy error")
	}
	if !strings.Contains(err.Error(), "deploy boom") {
		t.Errorf("runEC2() error = %v, want deploy boom", err)
	}
}

func TestRunEC2DeploySuccess(t *testing.T) {
	globals.SetGlobals(t, testEC2Cfg(), globals.WithDryRun(false))
	saveDeployFlags(t)
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "ec2", deployResult: &deploy.DeployResult{TargetName: "ec2", Status: "ACTIVE", Detail: "fleet-ec2"}}, nil
	})
	if err := runEC2(newCommand(), nil); err != nil {
		t.Fatalf("runEC2() error = %v", err)
	}
}

func TestRunEC2DeploySuccessNonSession(t *testing.T) {
	globals.SetGlobals(t, testEC2Cfg(), globals.WithDryRun(false))
	saveDeployFlags(t)
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &nonSessionTarget{name: "ec2"}, nil
	})
	if err := runEC2(newCommand(), nil); err != nil {
		t.Fatalf("runEC2() error = %v", err)
	}
}

func TestRunEC2DeploySuccessWithSession(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, testEC2Cfg(), globals.WithDryRun(false))
	saveDeployFlags(t)
	withSession = true
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "ec2", sessionInfo: &deploy.SessionInfo{SessionID: "sess-ec2", IPAddress: "9.9.9.9", Port: 7777}}, nil
	})
	if err := runEC2(newCommand(), nil); err != nil {
		t.Fatalf("runEC2() error = %v", err)
	}
}

func TestRunEC2DeploySuccessSessionError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, testEC2Cfg(), globals.WithDryRun(false))
	saveDeployFlags(t)
	withSession = true
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "ec2", sessionErr: fmt.Errorf("session boom")}, nil
	})

	err := runEC2(newCommand(), nil)
	if err == nil {
		t.Fatal("runEC2() expected session error")
	}
	if !strings.Contains(err.Error(), "session boom") {
		t.Errorf("runEC2() error = %v, want session boom", err)
	}
}

func TestRunAnywhereDryRun(t *testing.T) {
	globals.SetGlobals(t, testAnywhereCfg(), globals.WithDryRun(true))
	saveDeployFlags(t)
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, override string) (deploy.Target, error) {
		if override != "anywhere" {
			t.Errorf("runAnywhere() resolved target override %q, want anywhere", override)
		}
		return &fakeTarget{name: "anywhere"}, nil
	})
	if err := runAnywhere(newCommand(), nil); err != nil {
		t.Fatalf("runAnywhere() dry-run error = %v", err)
	}
}

func TestRunAnywhereResolveError(t *testing.T) {
	globals.SetGlobals(t, testAnywhereCfg(), globals.WithDryRun(true))
	saveDeployFlags(t)
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return nil, fmt.Errorf("resolve boom")
	})
	if err := runAnywhere(newCommand(), nil); err == nil {
		t.Fatal("runAnywhere() expected resolve error")
	}
}

func TestRunAnywhereDeployError(t *testing.T) {
	globals.SetGlobals(t, testAnywhereCfg(), globals.WithDryRun(false))
	saveDeployFlags(t)
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "anywhere", deployErr: fmt.Errorf("deploy boom")}, nil
	})

	err := runAnywhere(newCommand(), nil)
	if err == nil {
		t.Fatal("runAnywhere() expected deploy error")
	}
	if !strings.Contains(err.Error(), "deploy boom") {
		t.Errorf("runAnywhere() error = %v, want deploy boom", err)
	}
}

func TestRunAnywhereDeploySuccess(t *testing.T) {
	globals.SetGlobals(t, testAnywhereCfg(), globals.WithDryRun(false))
	saveDeployFlags(t)
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "anywhere"}, nil
	})
	if err := runAnywhere(newCommand(), nil); err != nil {
		t.Fatalf("runAnywhere() error = %v", err)
	}
}

func TestRunAnywhereDeploySuccessNonSession(t *testing.T) {
	globals.SetGlobals(t, testAnywhereCfg(), globals.WithDryRun(false))
	saveDeployFlags(t)
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &nonSessionTarget{name: "anywhere"}, nil
	})
	if err := runAnywhere(newCommand(), nil); err != nil {
		t.Fatalf("runAnywhere() error = %v", err)
	}
}

func TestRunAnywhereDeploySuccessWithSession(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, testAnywhereCfg(), globals.WithDryRun(false))
	saveDeployFlags(t)
	withSession = true
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "anywhere", sessionInfo: &deploy.SessionInfo{SessionID: "sess-any", IPAddress: "10.0.0.9", Port: 7777}}, nil
	})
	if err := runAnywhere(newCommand(), nil); err != nil {
		t.Fatalf("runAnywhere() error = %v", err)
	}
}

func TestRunAnywhereDeploySuccessSessionError(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, testAnywhereCfg(), globals.WithDryRun(false))
	saveDeployFlags(t)
	withSession = true
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "anywhere", sessionErr: fmt.Errorf("session boom")}, nil
	})

	err := runAnywhere(newCommand(), nil)
	if err == nil {
		t.Fatal("runAnywhere() expected session error")
	}
	if !strings.Contains(err.Error(), "session boom") {
		t.Errorf("runAnywhere() error = %v, want session boom", err)
	}
}
